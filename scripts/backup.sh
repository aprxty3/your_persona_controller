#!/usr/bin/env sh
# scripts/backup.sh — daily Postgres backup, uploaded to Cloudflare R2 (S3-compatible).
#
# Prerequisites (must be installed on the host that runs this script):
#   - pg_dump (Postgres client tools, matching the server's major version)
#   - aws CLI v2 (works against R2 via --endpoint-url, since R2 speaks the S3 API)
#   - gzip
#
# Configuration: reads DB_* / S3_* from the repo's .env (same vars the Go app
# itself uses — see .env.example) instead of inventing new ones. Only backup
# behavior (retention counts, R2 key prefixes) is script-local; those aren't
# runtime app config so they intentionally do NOT live in .env.example.
#
# Usage:
#   ./scripts/backup.sh [path/to/.env]     # defaults to ../.env relative to this script
#
# Suggested crontab entry (run daily at 02:00 WIB / 19:00 UTC):
#   0 19 * * * /opt/your-persona/controller-api/scripts/backup.sh >> /var/log/yp-backup.log 2>&1
# If your cron implementation supports a TZ= prefix, this is equivalent and
# easier to read:
#   TZ=Asia/Jakarta 0 2 * * * /opt/your-persona/controller-api/scripts/backup.sh >> /var/log/yp-backup.log 2>&1
#
# Restore drill: see docs/restore_drill.md — "a backup that's never been
# restore-tested is not a backup you can rely on."

set -eu

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
ENV_FILE="${1:-$SCRIPT_DIR/../.env}"

RETENTION_DAILY_DAYS="${BACKUP_RETENTION_DAILY_DAYS:-7}"
RETENTION_WEEKLY_DAYS="${BACKUP_RETENTION_WEEKLY_DAYS:-28}"
S3_PREFIX_DAILY="backups/daily"
S3_PREFIX_WEEKLY="backups/weekly"

log() {
	echo "[backup.sh] $(date -u +%Y-%m-%dT%H:%M:%SZ) $*"
}

fail() {
	log "ERROR: $*"
	exit 1
}

# ---------------------------------------------------------------------------
# Load configuration
# ---------------------------------------------------------------------------
if [ -f "$ENV_FILE" ]; then
	# shellcheck disable=SC1090
	set -a
	. "$ENV_FILE"
	set +a
else
	log "WARNING: env file $ENV_FILE not found — relying on already-exported environment variables"
fi

: "${DB_HOST:?DB_HOST is required (set in .env or environment)}"
: "${DB_PORT:=5432}"
: "${DB_USER:?DB_USER is required}"
: "${DB_PASSWORD:?DB_PASSWORD is required}"
: "${DB_NAME:?DB_NAME is required}"
: "${S3_ENDPOINT:?S3_ENDPOINT is required}"
: "${S3_BUCKET:?S3_BUCKET is required}"
: "${S3_ACCESS_KEY:?S3_ACCESS_KEY is required}"
: "${S3_SECRET_KEY:?S3_SECRET_KEY is required}"
: "${S3_REGION:=auto}"

for bin in pg_dump gzip aws; do
	command -v "$bin" >/dev/null 2>&1 || fail "required binary '$bin' not found in PATH"
done

# ---------------------------------------------------------------------------
# pg_dump — via the standard libpq env vars, so no flag/quoting gymnastics
# ---------------------------------------------------------------------------
export PGHOST="$DB_HOST"
export PGPORT="$DB_PORT"
export PGUSER="$DB_USER"
export PGPASSWORD="$DB_PASSWORD"
export PGDATABASE="$DB_NAME"

TIMESTAMP=$(date -u +%Y%m%dT%H%M%SZ)
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

DUMP_FILE="$WORKDIR/${DB_NAME}-${TIMESTAMP}.sql.gz"

log "dumping database '$DB_NAME' from $DB_HOST:$DB_PORT"
pg_dump --no-owner --no-privileges | gzip -9 > "$DUMP_FILE" \
	|| fail "pg_dump/gzip failed"

DUMP_SIZE=$(wc -c < "$DUMP_FILE" | tr -d ' ')
log "dump complete: $DUMP_FILE ($DUMP_SIZE bytes)"

# ---------------------------------------------------------------------------
# Upload — translate S3_* (this repo's naming) into AWS_* (aws CLI's naming);
# R2 is S3-compatible so --endpoint-url is the only extra flag needed.
# ---------------------------------------------------------------------------
export AWS_ACCESS_KEY_ID="$S3_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$S3_SECRET_KEY"
export AWS_DEFAULT_REGION="$S3_REGION"

s3_cp() {
	aws s3 cp "$1" "s3://${S3_BUCKET}/$2" --endpoint-url "$S3_ENDPOINT" --only-show-errors
}

DAILY_KEY="${S3_PREFIX_DAILY}/${DB_NAME}-${TIMESTAMP}.sql.gz"
log "uploading to s3://${S3_BUCKET}/${DAILY_KEY}"
s3_cp "$DUMP_FILE" "$DAILY_KEY" || fail "upload to daily/ failed"

# Sunday (ISO weekday 7) also gets a copy under weekly/, giving the 4-weekly
# / 7-daily rotation the ticket asks for without a second pg_dump run.
if [ "$(date -u +%u)" = "7" ]; then
	WEEKLY_KEY="${S3_PREFIX_WEEKLY}/${DB_NAME}-${TIMESTAMP}.sql.gz"
	log "Sunday — also uploading to s3://${S3_BUCKET}/${WEEKLY_KEY}"
	s3_cp "$DUMP_FILE" "$WEEKLY_KEY" || fail "upload to weekly/ failed"
fi

# ---------------------------------------------------------------------------
# Retention — delete objects older than the configured window per prefix.
# ---------------------------------------------------------------------------
prune_prefix() {
	prefix="$1"
	max_age_days="$2"
	cutoff_epoch=$(( $(date -u +%s) - max_age_days * 86400 ))

	log "pruning s3://${S3_BUCKET}/${prefix}/ older than ${max_age_days}d"

	aws s3api list-objects-v2 \
		--bucket "$S3_BUCKET" \
		--prefix "${prefix}/" \
		--endpoint-url "$S3_ENDPOINT" \
		--query 'Contents[].[Key,LastModified]' \
		--output text 2>/dev/null |
	while IFS="$(printf '\t')" read -r key last_modified; do
		[ -z "$key" ] && continue
		object_epoch=$(date -u -d "$last_modified" +%s 2>/dev/null) || continue
		if [ "$object_epoch" -lt "$cutoff_epoch" ]; then
			log "  deleting $key (last modified $last_modified)"
			aws s3 rm "s3://${S3_BUCKET}/${key}" --endpoint-url "$S3_ENDPOINT" --only-show-errors \
				|| log "  WARNING: failed to delete $key, will retry next run"
		fi
	done
}

prune_prefix "$S3_PREFIX_DAILY" "$RETENTION_DAILY_DAYS"
prune_prefix "$S3_PREFIX_WEEKLY" "$RETENTION_WEEKLY_DAYS"

log "backup complete"

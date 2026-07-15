# Restore Drill — Database Backup Recovery

> "A backup that's never been restore-tested is not a backup you can rely on." — PRD Section 7

This is a runbook, not an essay. Run it for real (against a scratch database, never production) at least once per quarter and after any change to `scripts/backup.sh` or the Postgres schema.

## Prerequisites

Same as `scripts/backup.sh`: `aws` CLI v2, `gzip`, and `psql` (Postgres client tools matching the server's major version). Credentials for the R2 bucket (`S3_*` in `.env`).

## 1. Find the backup to restore

```sh
export AWS_ACCESS_KEY_ID="$S3_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$S3_SECRET_KEY"

# List recent daily backups, newest last
aws s3 ls "s3://${S3_BUCKET}/backups/daily/" --endpoint-url "$S3_ENDPOINT"

# Or weekly, if testing longer-retention recovery
aws s3 ls "s3://${S3_BUCKET}/backups/weekly/" --endpoint-url "$S3_ENDPOINT"
```

## 2. Download and decompress

```sh
aws s3 cp "s3://${S3_BUCKET}/backups/daily/psyche_assessment-20260715T190000Z.sql.gz" \
	./restore-drill.sql.gz --endpoint-url "$S3_ENDPOINT"

gunzip restore-drill.sql.gz
```

## 3. Restore into a SCRATCH database — never production, never staging with real user data

```sh
# Create an empty, disposable database
createdb -h localhost -U postgres restore_drill_test

# Restore into it
psql -h localhost -U postgres -d restore_drill_test -f restore-drill.sql
```

If your local Postgres is the one from `docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml up`, connect via the exposed `localhost:5432` port with the `DB_USER`/`DB_PASSWORD` from `.env`.

## 4. Verify the restore actually worked

Don't just check that `psql` exited 0 — spot-check that data is really there:

```sql
-- Row counts on the core tables should be non-zero (adjust to whatever's
-- plausible for the backup's age)
SELECT 'users' AS table_name, count(*) FROM users
UNION ALL SELECT 'test_results', count(*) FROM test_results
UNION ALL SELECT 'answers', count(*) FROM answers
UNION ALL SELECT 'guest_sessions', count(*) FROM guest_sessions;

-- Spot-check a recent row's timestamp is sane (matches backup age, not
-- suspiciously old or in the future)
SELECT id, created_at FROM test_results ORDER BY created_at DESC LIMIT 5;
```

Checklist:
- [ ] `psql` restore completed without errors
- [ ] Row counts on `users`, `test_results`, `answers`, `guest_sessions` are non-zero and plausible
- [ ] Most recent `test_results.created_at` is consistent with the backup's timestamp (not older than expected — would indicate a stale/corrupt dump)
- [ ] No foreign-key constraint errors during restore (would indicate a partial/corrupt dump)

## 5. Clean up

```sh
dropdb -h localhost -U postgres restore_drill_test
rm restore-drill.sql
```

## If the drill fails

Treat it as a P1, not a documentation gap: a backup that can't restore is equivalent to no backup at all. Check, in order:
1. Was the backup file itself truncated/corrupted (compare `wc -c` against the size logged by `backup.sh` at upload time)?
2. Did `pg_dump`'s Postgres client version match the server's major version at dump time?
3. Was the R2 upload actually complete (no `aws s3 cp` errors in `backup.sh`'s log, e.g. `/var/log/yp-backup.log` per the crontab in `scripts/backup.sh`)?

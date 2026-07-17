# Deploy Runbook — Production (`api.yourpersonas.com`)

This is a runbook, not an essay — follow it top to bottom for a fresh deploy. A second person should be able to run this from nothing but this file and a fresh VM, without asking questions.

## Prerequisites (do these once, before touching the VM)

These are external/manual — nothing here is code, and none of it can be verified from a dev machine:

1. **DNS**: point `api.yourpersonas.com` at the VM's public IP (an `A`/`AAAA` record). Caddy (below) needs this resolvable before it can issue a TLS certificate.
2. **Cloudflare R2**: create the production bucket, generate an S3-compatible API token (`S3_ACCESS_KEY`/`S3_SECRET_KEY`), note the endpoint URL. Then, in the bucket's dashboard:
   - Enable **Object Lifecycle rule**: prefix `guest/` → expire after 14 days. Prefix `member/` gets **no** lifecycle rule. (See `TECHNICAL_DOCUMENTATION.md` Section 6.1 for why — orphan-PDF safety net, not a replacement for the purge job.)
   - **Attach a screenshot or exported rule config to the deploy PR** — this is a checklist item humans forget to verify later, and the setting lives entirely in a dashboard with no code trail otherwise.
3. **Brevo**: create/verify the sending domain (SPF + DKIM records on your DNS provider), generate SMTP credentials under Settings → SMTP & API → SMTP.
4. **Cloudflare Turnstile**: create a site key + secret for `api.yourpersonas.com` (or the FE domain, per Turnstile's widget setup) in the Cloudflare dashboard. This is `TURNSTILE_SECRET_KEY`.
5. Have SSH access to a fresh VM with Docker + Docker Compose v2 installed.

## 1. First-time deploy

```sh
# On the VM
git clone <repo-url> /opt/your-persona/controller-api
cd /opt/your-persona/controller-api

cp .env.example .env
# Edit .env — fill in EVERY value from Prerequisites above, plus:
#   APP_ENV=production
#   DB_PASSWORD=<generate a real random password, not "changeme">
#   JWT_SECRET=<generate with: openssl rand -base64 48>
#   ALLOWED_ORIGINS=https://app.yourpersonas.com   (the FE production origin, no wildcard)
#   TRUSTED_PROXIES=172.16.0.0/12                  (covers Docker's default bridge ranges;
#                                                    confirm the exact subnet with
#                                                    `docker network inspect your-persona-prod_default`
#                                                    after step 2 below, and narrow this if you want
#                                                    it exact rather than a broad private range)
# cmd/api and cmd/worker refuse to boot with any of the above still empty or
# at its dev default (internal/config.RequireProduction) — that's
# intentional, not a bug if it happens here.

docker compose -f docker/docker-compose.prod.yml --env-file .env up -d --build
```

```sh
# Run the migration MANUALLY — never auto-run at container boot
docker compose -f docker/docker-compose.prod.yml run --rm api ./migrate

# Seed the question bank + insight templates (idempotent, safe to re-run)
docker compose -f docker/docker-compose.prod.yml run --rm api ./seed
```

```sh
# Smoke test
curl -i https://api.yourpersonas.com/healthz
# → 200, {"database":"up","redis":"up"}, and a valid TLS cert (curl won't
#   complain, or check with: curl -v https://api.yourpersonas.com/healthz 2>&1 | grep -i "SSL certificate verify ok")
```

Then manually, from a browser or Swagger UI (`https://api.yourpersonas.com/swagger/index.html`):
- `POST /v1/auth/register` with a real email → confirm the OTP email actually arrives (Brevo, not Mailpit — this is the one thing that CANNOT be verified any other way).
- Full Guest submit path (`guest-session` → `submit`) → confirm a PDF appears in the R2 bucket under `guest/{session_id}/{result_id}.pdf`.

## 2. Register the backup cron

`scripts/backup.sh` and its crontab line are already written — this step just activates it on this VM:

```sh
which pg_dump && which aws   # both must be installed on the HOST (not just in the api container)
crontab -e
# Add the line documented in scripts/backup.sh's own header comment:
#   0 19 * * * /opt/your-persona/controller-api/scripts/backup.sh >> /var/log/yp-backup.log 2>&1

# Verify immediately — don't wait for 02:00 WIB to find out it's broken:
/opt/your-persona/controller-api/scripts/backup.sh
aws s3 ls "s3://${S3_BUCKET}/backups/daily/" --endpoint-url "$S3_ENDPOINT"
# → the .sql.gz this manual run just produced should be listed
```

Postgres is bound to `127.0.0.1:5432` in `docker-compose.prod.yml` specifically so this host-run script can reach it via `DB_HOST=localhost` (same as dev) — it is not reachable from outside the VM.

## 3. Verify trusted-proxy IP extraction actually works

```sh
# From two different networks (e.g. your laptop's wifi + phone hotspot),
# hit an endpoint that's rate-limited and check the app logs —
# each network should get its own rate-limit bucket, not share one.
curl -X POST https://api.yourpersonas.com/v1/guest-session -d '{...}'
docker compose -f docker/docker-compose.prod.yml logs api | grep "rate_limited\|guest session created"
```

If both networks appear to share the same bucket (11th request from network #2 also gets `429` after only a handful of combined requests), `TRUSTED_PROXIES` doesn't match Caddy's actual container IP/subnet — re-check with `docker network inspect`.

## 4. Verify graceful shutdown

```sh
# Trigger a redeploy (step 5) while a PDF generation job is in flight, then:
docker compose -f docker/docker-compose.prod.yml logs worker | tail -20
# → should show "interrupt signal received, shutting down ... gracefully" followed
#   by the in-flight job completing, NOT an abrupt kill mid-job.
```

## 5. Redeploy (routine updates)

```sh
cd /opt/your-persona/controller-api
git pull
docker compose -f docker/docker-compose.prod.yml --env-file .env up -d --build
# If this release includes a migration:
docker compose -f docker/docker-compose.prod.yml run --rm api ./migrate
```

`docker compose up -d` recreates changed containers one at a time; `api`/`worker` both handle `SIGTERM` gracefully (`SHUTDOWN_TIMEOUT` in `.env`, default 30s) — in-flight HTTP requests and Asynq jobs finish before the old container exits, not before the new one is ready to receive traffic. No separate "drain" step needed.

## 6. Rollback

```sh
cd /opt/your-persona/controller-api
git log --oneline -5           # find the last known-good commit
git checkout <commit-sha>
docker compose -f docker/docker-compose.prod.yml --env-file .env up -d --build
```

If the bad release included a migration that's hard to reverse (dropped/renamed a column), rolling back code without also rolling back schema will break — check `cmd/migrate`'s migration list before rolling back across a migration boundary. There is no automated down-migration in this codebase — a schema rollback is a manual `ALTER TABLE`, not a tool command.

## Reference — what's already handled by code, not this runbook

- Cookie `Secure`/`SameSite=Strict` — `APP_ENV=production` is the only switch needed.
- CORS wildcard rejection, CSRF double-submit — already enforced in code, nothing to configure beyond `ALLOWED_ORIGINS`.
- Rate limiting, garbage-input filtering — already enforced in code, just needs `TRUSTED_PROXIES` correctly set for the rate limit to attribute requests to the right client (this runbook, step 1/3).

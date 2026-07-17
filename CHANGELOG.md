# CHANGELOG — controller-api

Format: [Semantic Versioning](https://semver.org/) — **[UNRELEASED]** means not yet tagged/released.
Conventions: `[A]` Added · `[C] `Changed · `[F]` Fixed · `[D]` Deprecated · `[R]` Removed

---

## [UNRELEASED] — 2026-07-17 (2)

### Referral stats endpoint & insight template content expansion (TICKET-25, TICKET-24)

#### [A] `GET /v1/account/referral-stats` — aggregate-only referral conversion counts (TICKET-25)
- New `ProfileUseCase.GetReferralStats(ctx, userID)` — reuses the existing `ProfileUseCase` (no new package, serumpun `GetReferralCode`) and the already-implemented (previously zero-caller) `ReferralRepository.CountEventsByCodeID`. No wiring/constructor changes.
- Response is `{ code, signup_count, completed_count }` — **aggregate only, by design (UU PDP)**: never invitee email/name/user_id. A user who hasn't generated a code yet gets `200` with zero counts and an empty code, not `404` or an auto-generated code — code generation stays `GetReferralCode`'s exclusive responsibility.
- Test coverage explicitly enumerates the response's allowed key set (`code`/`signup_count`/`completed_count`) rather than spot-checking fields, so an accidental future PII field fails the test immediately.
- Swagger annotations added; `make swag` still needs to be run by the user per standing convention (not run here).

#### [A] Insight template content — GRIT decrease + first-pole MBTI strengths/blind-spots (TICKET-24, FR-E2)
- Seeder (`cmd/seed/main.go`) grows from 2 templates (GRIT only) to 18 (9 `insight_key` × EN/ID): `grit_decrease` (mirrors `grit_increase`, dashboard-only), plus 1 strength + 1 blind-spot template per **first pole** of each MBTI dimension (E/S/T/J at trait score ≥ 60).
- **Scope deliberately narrower than the ticket's original ask, per user decision after a flagged gap**: `condition_type=threshold` in both consumers (`GeneratePDFUseCase.collectInsights`, `user_dashboard`'s micro-insight) only evaluates `value >= ThresholdValue` — there's no "below threshold" path. The ticket's second-pole templates (I/N/F/P, low score) and `grit_low_threshold` need that semantic and can't be safely seeded with current logic — a naive low `ThresholdValue` would fire on almost every score, showing the wrong pole's text to most users. Deferred to a follow-up ticket that adds a deliberate schema decision (e.g. a `direction` field) rather than smuggled into this content-only ticket. See `ticket-24-insight-template-content.md`'s "Catatan implementasi" for the full breakdown.
- PRD FR-E2 note added documenting the scoped delivery and the deferred second-pole gap.

## [UNRELEASED] — 2026-07-17

### General per-IP rate limiting & garbage input detection (TICKET-22, TICKET-23)

#### [A] `POST /v1/guest-session` and `POST /v1/assessment/submit` now rate-limited per-IP (TICKET-22)
- `IPRateLimitService` (`internal/infrastructure/cache/redis/ip_rate_limit.go`) gains two scopes: `guest-session` (10x/hour/IP) and `submit` (10x/hour/IP) — budgeted separately from `login`/`register`, closing the cheapest Denial-of-Wallet loop (mint a fresh guest session to bypass the per-session quota lock, then submit, repeat).
- Checked at the very top of `CreateGuestSessionUseCase.Execute` (before validation/DB) and `SubmitAssessmentUseCase.Execute` (before idempotency/lock/DB — reject cheap before expensive work), same placement convention as `RegisterUseCase.Register`/`SessionUseCase.Login`. Fail-open on a Redis error (`Warn` log, request proceeds) — same graceful-degradation rule as the existing login/register limiters.
- **Architectural note**: `SubmitAssessmentUseCase` can't reuse `auth.IPRateLimiter` or `redis.IPScope` directly — `internal/infrastructure/cache/redis` already imports `internal/application/assessment` (for `IdempotencyService`/`DistributedLockService`), so `assessment` importing `redis` back would cycle. Fixed with: a package-local `assessment.IPRateLimiter` interface parameterized on `dto.IPRateLimitScope` (lives in the same leaf `dto` subpackage as `SubmitResponse`, for the identical import-cycle reason documented there) instead of `redis.IPScope`; and a thin `assessmentIPRateLimiterAdapter` in `cmd/api/wire.go` — the one place both packages are already visible together — that converts to `redis.IPScope` and satisfies `assessment.IPRateLimiter` via `*redis.IPRateLimitService`.
- `rateLimitedResponse` moved from `auth_handler.go` to `helpers.go` now that `assessment_handler.go` needs the same 429 shape.
- Deliberately NOT rate-limited (KISS, per ticket): light read endpoints (`GET /v1/questions`, `GET /v1/results/:id`, dashboard) — no abuse evidence yet, BodyLimit + auth guard already sufficient.

#### [A] Garbage input detection skips the Gemini call, never blocks submit (TICKET-23, FR-B5)
- New `pkg/aivalidator.IsGarbage(text string) bool` — length floor (30 chars), unique-rune ratio, non-letter ratio, and longest-unbroken-word heuristics, all named constants for easy recalibration. No NLP dependency (KISS), consistent with `ValidateOutput`'s existing pure-function style in the same package.
- `SubmitAssessmentUseCase.Execute` filters each essay through `IsGarbage` right before the AI phase — essays that fail are logged (index only, no content) and excluded from the Gemini call; essays that pass go through unchanged. All-garbage reuses the existing no-essay `fallback_static` path (zero new status/branch). Every essay, garbage or not, is still stored verbatim in `ANSWER` and still scanned for wellbeing/crisis language BEFORE garbage filtering — a distressed-but-repetitive answer must never skip the safety net just because it also reads as garbage.
- PRD FR-B5 → `[~]` (backend done; FE indicator still pending, that's FR-B5's own remaining scope).

#### Test coverage
- `pkg/aivalidator/garbage_test.go`: table-driven, including the ticket-mandated false-positive guards (EN/ID short-but-legit essays ~50 chars, emoji-laden normal text) alongside repeated-character/symbol-mash/keyboard-mash/empty-string positives.
- `submit_assessment_test.go`: `filterGarbageEssays` pure-selection tests (all-garbage, all-legit, mixed, empty); rate-limit-gate tests (rejects before idempotency check, Redis error fails open).
- `create_guest_session_test.go`: rate-limit-gate tests (rejects before validation/repo call, Redis error fails open).
- Handler-level: `TestSubmit_RateLimited_429`, `TestCreateGuestSession_RateLimited_429` — status code, `RATE_LIMITED` code, `meta.retry_after_seconds` presence.
- `go build`/`go vet`/`go test` clean across every package except `cmd/api` (its `wire_gen.go` needs `make wire` regenerated for the 2 new constructor params — left for the user per standing convention; `wire.go`, the injector source, already updated).

## [UNRELEASED] — 2026-07-16

### Full `internal/application` test coverage, migrated to mockery (post-TICKET-21 follow-up)

#### [A] Every use case in `internal/application` now has unit tests; hand-written mocks migrated to mockery
- Adopted [mockery](https://github.com/vektra/mockery) (`.mockery.yaml`, `with-expecter: true`) for every repository/service interface across `internal/domain/{account,content,deletionrequest,testresult}`, `internal/application/{assessment,auth,deletionrequest,guestpurge,pdf,user_dashboard}`, and `pkg/taskqueue` — generated mocks are checked into git (same convention as `wire_gen.go`). New direct dependency: `github.com/stretchr/testify` (was already an indirect transitive dependency).
- Two packages (`assessment`, `pdf`) generate `inpackage: true` mocks into `mock_*_test.go` files instead of a `mocks` subpackage — `assessment.IdempotencyService` and `pdf.PDFRenderer` both reference a type defined in their own package (`SubmitResponse`, `PDFData`), and a subpackage mock importing back would hit Go's "import cycle not allowed in test" as soon as that package's own internal test files import it.
- **New testability refactor**: `IPRateLimiter`/`OTPRateLimiter` interfaces (`internal/application/auth/rate_limiter.go`) replace the concrete `*redis.IPRateLimitService`/`*redis.OTPRateLimitService` fields on `SessionUseCase`/`RegisterUseCase`/`AccountUseCase` — same pattern as TICKET-21's `SessionTokenStore`, non-breaking (2 new `wire.Bind` lines).
- **Converted to mockery**: `submit_assessment_test.go`, `session_test.go`, `account_test.go`, `user_dashboard_test.go`.
- **New test files**: `assessment/result_query_test.go`, `assessment/question_catalog_test.go`, `assessment/wellbeing_test.go`, `auth/register_test.go`, `auth/create_guest_session_test.go`, `auditpurge/purge_audit_ttl_test.go`, `deletionrequest/deletionrequest_test.go`, `deletionrequest/anonymize_test.go`, `guestpurge/purge_guest_ttl_test.go`, `pdf/generate_pdf_test.go`, `profile/profile_test.go` — also adds Login/VerifyEmailOTP/LogoutAll/Logout coverage to `SessionUseCase` that TICKET-21 didn't require.
- **Consistent boundary rule**: any use case calling `uc.db.Transaction(...)` with a concrete Postgres constructor inside the closure (`RegisterUseCase.Register`, `SessionUseCase.ResetPassword`/`ChangePassword`, `AnonymizeUseCase.Anonymize`, `PurgeGuestTTLUseCase.Execute`'s per-row transaction) is unit-tested only up to that boundary — the transaction body needs a real DB and belongs to integration tests, not unit tests (AGENTS.md's own rule). Every such test file marks this boundary explicitly in comments.
- `go build ./...`, `go vet ./...`, and `go vet -tags=integration ./...` all clean. `go test ./...` itself left for the user to run (standing instruction).

### Dashboard Micro-Insight & Test Suite (TICKET-20, TICKET-21)

#### [A] Dashboard micro-insight, rule-based, no Gemini (TICKET-20, FR-F4)
- `GET /v1/user-dashboard` gains `micro_insights []string`. Evaluates the GRIT dimension only against `INSIGHT_TEMPLATE` rows — reuses `content.InsightTemplateRepository.FindMatchingTemplates` (the same repository the PDF worker already consumes for Strengths/Blind Spots), newly wired into `cmd/api` (previously only reachable from `cmd/worker`).
- Three `condition_type`s evaluated: `increase`/`decrease` (delta between the 2 most recent results vs `min_delta`, `{delta}` substituted) and `threshold` (latest result vs `threshold_value`, `{value}` substituted, needs only 1 result). Delta is derived from the same `FindHistoryByUser` call `GetDashboard` already makes for `grit_trend` — no extra query.
- Guards against fabricating insights from incomplete/pre-scoring data: fewer than 2 results skips delta entirely; either delta-input result having `grit_score == 0` (a pre-TICKET-17 row, not a real zero) skips delta; a `0` latest score also can't satisfy a threshold. `micro_insights` is always `[]`, never `null`, when empty.
- `DashboardUseCase.GetDashboard` gains a `locale` parameter, resolved in the handler via `middleware.LocaleFromContext(c)` (same pattern as `assessment_handler.go`/`result_handler.go`) — fallback-to-EN is not duplicated here, `FindMatchingTemplates` already owns that (FR-I9).
- New `user_dashboard_test.go`: 10 unit tests covering the PRD's own worked example (70→76 fires, 70→74 doesn't), threshold-with-single-result, both zero-score guards, empty-history, and template-lookup-error propagation.

#### [A] Test suite — security-critical unit tests + testcontainers-go integration tests (TICKET-21)
- **Unit tests** (mocked repositories, no DB/Redis — `internal/application/...`, `internal/infrastructure/security`, `internal/infrastructure/i18n`, `internal/interfaces/http`):
  - OTP attempt-limit gate (`validateOTPAttempt`): 5th wrong guess rejects with `ErrOTPMaxAttempts`; any further attempt — including the correct code — stays rejected without incrementing further.
  - `ResetPassword` single-use contract: a jti Redis already returned empty (replay) rejects with `ErrInvalidToken` without touching the DB transaction; subject-mismatch defense-in-depth; malformed JWT rejects before ever calling `ConsumeResetJTI`.
  - `RefreshToken`: stale `token_version` rejects with `ErrTokenVersionMismatch`; denylisted jti rejects with `ErrInvalidToken`.
  - `SubmitAssessmentUseCase.Execute`: idempotency-key-reused and cache-hit both return before the lock/Gemini are ever touched; lock-not-acquired; Member and Guest quota-exceeded (post-TICKET-19) both reject and still release the lock.
  - `runAIPhase` (isolated from the DB-bound rest of `Execute`): no-essay skip, Gemini transport error → `fallback_static` (FR-C2, never surfaces as an error), invalid/too-short output → `fallback_static`, valid output → `completed`.
  - Cloudflare Turnstile: explicit `success:false` fails closed; transport/5xx/timeout/malformed-JSON all fail open (TICKET-11 contract); empty secret key → noop verifier, zero HTTP calls.
  - i18n catalog: both locales load; unsupported/empty locale falls back to EN content (FR-I9); unknown purpose → `ok=false`.
  - `ParseAllowedOrigins`: trims/splits/skips-empty; a bare `"*"` or `"*"` among other origins panics (TICKET-14 contract, now test-locked).
- **Refactors enabling the above** (no behavior change): `turnstileClient` gained an injectable `verifyURL` field (test-only override, deliberately not an env var — production callers still hit the real Cloudflare constant). `SessionUseCase.tokenStore` changed from the concrete `*redis.TokenStore` to a new narrow interface `auth.SessionTokenStore` (same 4 methods) — application-layer tests can now fake Redis token bookkeeping instead of needing a live connection; `*redis.TokenStore` still satisfies it, so `cmd/api` wiring is unaffected beyond an added `wire.Bind`.
- **Integration tests** (new dependency `github.com/testcontainers/testcontainers-go` + `.../modules/postgres` + `.../modules/redis`, gated behind `//go:build integration` so they never slow down `make test`):
  - Postgres (`internal/infrastructure/persistence/postgres/assessment/integration_test.go`, one shared container via `TestMain`): `UpsertAnswers` ON CONFLICT collapses a re-submitted question to 1 row; `CountMonthlyUsage` correctly buckets a row sitting exactly on the Asia/Jakarta month boundary (17:00 UTC on the last day of the prior month) vs. one second earlier; `ScrubEssayAnswersByUser` blanks only essay-type answers, Likert survives; `PromptAuditLogRepository.DeleteByUserID`'s `test_result_id IN (subquery on user_id)` scoping deletes only the target user's logs; the partial unique index `uniq_active_deletion_per_user` rejects a second `pending_grace`/`processing` row for the same user (`gorm.ErrDuplicatedKey`) but allows one once the first is `cancelled`.
  - Redis (`internal/infrastructure/cache/redis/integration_test.go`, one shared container via `TestMain`): `ConsumeResetJTI` under 2 concurrent goroutines on the same jti — exactly 1 winner (real Redis `GETDEL` atomicity, not simulated); `ReleaseLock`'s CAS Lua script refuses to delete a key whose value changed underneath it (simulated new owner) but does delete on a matching token; `IdempotencyService.Check`/`Save` round-trip and cache-miss/hash-mismatch paths against real Redis.
- **README**: `## Testing` section now documents the real `make test` vs `go test -tags=integration ./...` split and the Docker requirement for the latter; CI still runs unit-only (`make test`), integration tag intentionally not wired into CI yet (would need Docker-in-Docker on the runner — tracked as a follow-up, not blocking this ticket).

## [UNRELEASED] — 2026-07-15

### Scoring Engine, Retention Sweeps, Guest Quota (TICKET-17, TICKET-18, TICKET-19)

#### [C] `GET /v1/questions` no longer exposes scoring metadata (contract change, pre-FE-integration)
- `is_reverse_scored` and `is_attention_check` removed from the public question-bank response (present since TICKET-04). Both are scoring internals, not rendering data: exposing which item is the attention check lets a bot always answer it correctly (defeating its entire purpose), and exposing scoring direction helps users game their MBTI/GRIT results. Found while deliberately keeping TICKET-17's new `trait`/`option_trait_map` columns (the literal answer key) out of this DTO — the two older fields fail the same test. FE renders every question identically and never needed them. Non-breaking in practice: no client consumes the API yet. Swagger regenerated.

#### [A] Scoring engine — MBTI type, GRIT score & trait scores now computed on every submit (TICKET-17)
- **Closes the biggest remaining backend gap**: `TEST_RESULT.mbti_type`/`grit_score`/`trait_scores` were zero-value since TICKET-03 (flagged in every downstream ticket). New pure function `assessment.ComputeScores` (`internal/application/assessment/scoring.go`) — no DB, no side effects; returns a `ScoreResult` struct (deliberate deviation from the ticket's 3-bare-returns signature: the struct also carries `NeutralFallbackDimensions` so the use case can Warn-log empty dimensions while the function itself stays pure/loggerless).
- **Schema**: `QuestionModel` gained `Trait varchar(10)` (Likert → one of EI/SN/TF/JP/GRIT) and `OptionTraitMap jsonb` (SJT → per-option signed points, positive = first pole). Mirrored (tagless) on domain entity `content.Question` + mapper. AutoMigrate picks the columns up; re-running the seeder backfills existing rows (upsert `DoUpdates` extended).
- **Seeder** (`cmd/seed/main.go`): traits + option maps seeded exactly per the ticket's mapping table, including the two `is_reverse_scored` corrections it mandates — `b16` (N-pole statement) and `b19` (classic reverse-keyed Duckworth Grit item; pre-existing data bug).
- **Formula** (per ticket, reproduced in unit tests as numeric contracts): Likert contributes `effective-3` (reverse: `6-value`), SJT contributes its chosen option's signed points; per-dimension percentage = `round(50 + sum/maxAbs*50)` clamped 0-100, letter = first pole at ≥ 50. GRIT = `round((avgEffective-1)/4*100)` over GRIT Likert items only (never mixed with SJT points). Attention checks excluded entirely; malformed values/options degrade to zero contribution; a dimension with no valid answers falls back to neutral 50 + Warn, never fails the submit.
- **Integration**: `SubmitAssessmentUseCase.Execute` fills all three fields before insert — `fallback_static` results get full scores too (scores don't depend on Gemini). Verified live: full guest submit with `GEMINI_API_KEY` unset → `ESTJ` / GRIT 88 / 5-key `trait_scores`, matching hand computation.
- New `scoring_test.go`: 8 table-driven tests including both worked examples from the ticket (EI=58→E, GRIT=88), reverse-scoring inversion, attention-check invariance, MBTI always-valid-4-letters, GRIT boundaries (0/100), empty-dimension fallback, malformed-SJT handling.
- **Companion fixes the ticket's "no PDF/dashboard changes needed" assumption turned out to require** (found by code analysis, verified against a generated PDF):
  - `GeneratePDFUseCase.collectInsights` never evaluated `condition_type` — with trait_scores now non-empty it would have printed every template unconditionally, including `grit_increase` with a raw unreplaced `{delta}`. Now: `threshold` templates require `value >= threshold_value` and get `{value}` substituted; `increase`/`decrease` are skipped (they need a previous result to diff — dashboard-trend scope, not this single-result job). Also normalizes the trait key (`GRIT` → `grit`) to match the template registry's lowercase convention.
  - `MarotoRenderer.addChartSection` drew a GRIT bar from `data.GritScore` AND would have drawn a second identical bar from the new `trait_scores["GRIT"]` key — the key is now skipped in the loop.

#### [A] Retention sweeps — `purge:audit-ttl`, expired result = 404, orphan guest sessions (TICKET-18)
- **`purge:audit-ttl`** (new task type in `pkg/taskqueue`): new `auditpurge.PurgeAuditTTLUseCase` calling `PromptAuditLogRepository.DeleteExpired` — the method existed since TICKET-09 with zero callers, so the 30-day raw-prompt TTL promise (PRD Section 8) was never actually kept. `DeleteExpired` signature widened to return `RowsAffected` for the run log. Registered `@daily` + boot-kick in `cmd/worker/main.go`, queue `low`, mirroring `purge:guest-ttl`. `PurgeHandler` now hosts both parameterless purge handlers (`ProcessPurge`/`ProcessAuditPurge`) rather than spawning a near-identical second handler struct.
- **Expired result = 404** (PRD Section 9.6): new shared `ResultUseCase.findVisible` helper — `FindByID` + `IsExpired()` → `ErrResultNotFound` — now feeds `GetByID` and `findOwned`, covering all four result paths (`GET /v1/results/:id`, mascot-style PATCH, pdf-status, pdf download) with one guard. `UpdateMascotStyle` was also refactored onto `findOwned` (it previously duplicated the lookup+ownership block inline). A guest result past its 14-day TTL is 404 even before the daily purge physically deletes the row (verified live by rewinding `expires_at` via SQL).
- **Orphan guest sessions**: `PurgeGuestTTLUseCase` now also sweeps expired, unclaimed sessions that never produced a test result (onboard-then-bail rows previously lived forever — TICKET-08 scope note). `GuestSessionRepository.FindExpiredUnclaimed` (also a zero-caller method until now) gained a `NOT EXISTS (test_results…)` guard per the ticket's safety warning: a session can expire while its result is still inside the result TTL (result TTL counts from submit, session TTL from creation) — those sessions belong to the result-driven purge path and are never touched by this sweep (verified live: orphan purged, session-with-live-result survived).

#### [A] Guest quota soft enforcement — 1×/month per session_id (TICKET-19)
- `SubmitAssessmentUseCase.Execute`'s quota block now has a Guest branch: new `TestResultRepository.CountMonthlyUsageByGuestSession` (same Asia/Jakarta month boundary, same `completed`+`fallback_static` statuses as the Member count — boundary computation extracted to a shared `startOfCurrentMonthJakarta()` so the two can never drift) checked against new `application.GuestMonthlyQuota = 1`, inside the existing `quota_lock:{session_id}` distributed lock. Second submit from the same session → `QUOTA_EXCEEDED` 429 (verified live); a fresh session after clearing the cookie still works — accepted by design (PRD Section 5 frames Guest quota as a nudge; this closes the same-session Denial-of-Wallet path, not cookie-clearing).
- Drive-by cleanup: `CountMonthlyUsage`'s `(user_id = ? OR guest_session_id = ?)` filter — the OR clause was dead (a user's UUID never appears as a `guest_session_id`; claim flows null the column) and misleading now that a real Guest counterpart exists; simplified to `user_id = ?`.

#### [F] CSRF cookie was never primed — `Skipper` fix (TICKET-14 follow-up, found during smoke test)
- TICKET-14's `Skipper` returned true (skip) for every non-protected path — but Echo skips **cookie priming too** when skipped, so `csrf_token` was never set by `GET /v1/questions` (or anything else), and the first protected POST could only fail 400 "missing csrf token". The router comment claimed priming worked; it didn't. Fix: safe methods (GET/HEAD/OPTIONS/TRACE) now always pass through the middleware (Echo never enforces on them, but does prime the cookie); unsafe methods outside `csrfProtectedPaths` still skip. Verified live: GET primes the cookie, submit with header passes, submit without header is rejected.

### Hardening & Ops — Turnstile, Email i18n externalization, DB Backup, CSRF/CORS (TICKET-11, TICKET-12, TICKET-13, TICKET-14)

#### [A] Cloudflare Turnstile bot verification on Register/Login/ForgotPassword (TICKET-11)
- New `auth.TurnstileVerifier` interface (declared in `internal/application/auth/account.go`, point-of-consumption per this codebase's convention) + `internal/infrastructure/security/turnstile.go` implementing it against Cloudflare's `siteverify` API — structurally mirrors `HIBPBreachChecker` (`httpClient *http.Client` + `log`, own 5s `context.WithTimeout` independent of the caller's ctx, constructor returns the interface type).
- **Fail-open on Cloudflare-side errors** (transport/timeout/non-200/decode failures): logs `Warn` and allows the request through, matching `HIBPBreachChecker`'s precedent — an outage degrades bot protection rather than taking down all registration/login/forgot-password traffic; per-IP rate limiting and account lockout remain the primary defenses. An explicit `success:false` verdict from Cloudflare still fails closed.
- **Dev bypass**: `NewTurnstileVerifier` returns a `noopTurnstileVerifier` (always passes) when `TURNSTILE_SECRET_KEY` is empty — encapsulated at construction time so no caller branches on "is dev mode" itself.
- New DTO field `cf_turnstile_response` (required) on `RegisterRequestDTO`/`LoginRequestDTO`/`ForgotPasswordRequestDTO`; verified via one shared `AuthHandler.verifyTurnstile` helper called right after `bindJSON` in all three handlers (ticket text asks for handler-level, not use-case-level, verification — this is an HTTP-layer/anti-automation concern, not a domain rule) — zero duplication across the 3 call sites.
- New sentinel `application.ErrTurnstileFailed` (`TURNSTILE_VERIFICATION_FAILED`, 400).
- `cmd/api/wire.go`/`main.go`: new `TurnstileSecretKey` typed alias reading `TURNSTILE_SECRET_KEY` (already present in `.env.example` as an unused placeholder since an earlier docs pass — now actually wired).

#### [C] Email i18n — templates externalized to JSON, locale wiring was already complete (TICKET-12)
- **Scope correction from the ticket's original text**: research before implementation found `Mailer.SendOTP`/`SendDeletionConfirmed` already accepted `locale`, already resolved it via `pkglocale.Resolve`, and all 3 enqueue call sites (register, resend-OTP, forgot-password) already populated `SendEmailPayload.Locale` from `user.PreferredLocale`. The only unmet acceptance criterion was "message catalog separate from Go code" — that's the entire scope actually executed here.
- New `internal/infrastructure/i18n` package: `locales/en.json`/`locales/id.json` (byte-for-byte content migrated from the old `mailer/templates.go` hardcoded maps — copy unchanged) embedded via `go:embed`, loaded once at startup (`LoadCatalog`, fails fast on malformed JSON rather than booting with silently-empty email bodies). `Catalog.Message(purpose, locale)` internally calls `pkglocale.Resolve` — reuses the existing fallback authority rather than introducing a second one (or a `go-i18n` dependency whose responsibility would fully overlap with `pkg/locale`, per AGENTS.md KISS/DRY).
- `internal/infrastructure/mailer/templates.go` deleted; `SMTPMailer` gained a `catalog *i18n.Catalog` constructor param. `Mailer` interface itself (`SendEmail`/`SendOTP`/`SendDeletionConfirmed`) is byte-for-byte unchanged, so `email_handler.go` and every enqueue call site needed zero changes.
- `cmd/worker/main.go`: loads `i18n.LoadCatalog()` once at boot (fail-fast on error, same posture as the catalog loader itself), passes it into `mailer.NewSMTPMailer`.

#### [A] `scripts/backup.sh` — daily Postgres backup to R2 with rotation (TICKET-13)
- POSIX shell script (not a Go binary — runs on the VM/host per the ticket's own phrasing, not inside the app process): `pg_dump` via standard libpq env vars (`PGHOST`/`PGUSER`/`PGPASSWORD`/`PGDATABASE`/`PGPORT`, sourced from this repo's own `DB_*` `.env` vars — no separate credential set to maintain) piped to `gzip`, uploaded via `aws` CLI with `S3_*` → `AWS_*` env-var translation + `--endpoint-url` (R2 is S3-compatible).
- Always uploads to `backups/daily/`; on Sundays (`date +%u == 7`) also duplicates to `backups/weekly/` — gives the 7-daily/4-weekly rotation without a second `pg_dump` run.
- Retention: lists each prefix via `aws s3api list-objects-v2`, deletes objects older than 7d (daily) / 28d (weekly). Per-object delete failures are logged and skipped (self-heals next run), matching this codebase's established purge-job idempotency pattern.
- New `docs/restore_drill.md` — runbook for downloading a backup, restoring into a scratch database, and verifying row counts/timestamps actually round-tripped. Not automated (deliberately manual/quarterly per PRD Section 7's "never-tested backup" warning) since there's no production VM yet to schedule it on (that's TICKET-26 scope).

#### [A] CSRF & CORS protection (TICKET-14)
- **CORS**: `middleware.CORSWithConfig` registered globally (right after Recover/Logger, before BodyLimit). `AllowOrigins` sourced from new `ALLOWED_ORIGINS` env var via new exported `http.ParseAllowedOrigins` — **panics at startup if any entry is a literal `"*"`**, enforcing the ticket's wildcard ban structurally rather than by convention alone. `AllowCredentials: true` (cookies must round-trip cross-origin to the separate `front-end/` app); `AllowHeaders` includes `X-CSRF-Token` and the pre-existing `Idempotency-Key`.
- **CSRF**: `middleware.CSRFWithConfig` registered globally with a `Skipper` — new `csrfProtectedPaths` allowlist (`router.go`) enforces the token check on exactly the 3 route categories AGENTS.md's Security Rules already name ("submit tes, deletion request, ubah preferensi locale": `/v1/assessment/submit`, `/v1/account/profile`, `/v1/account/delete-request`, `/v1/account/delete-request/cancel`) — NOT globally on every mutating endpoint. Login/register are deliberately excluded: they're Bearer-token (not ambient-cookie) authenticated, so CSRF isn't the relevant threat model there — Turnstile (TICKET-11) + per-IP rate limiting cover that surface instead. Registering the middleware globally (rather than per-group) means the `csrf_token` cookie gets primed on any request (even a plain `GET /v1/questions`), so it's already set by the time the frontend needs to submit a protected request.
- `SameSite=Lax` chosen over `None` for the CSRF cookie: sufficient because front-end and this API are expected to share a parent domain (subdomains, e.g. `app.yourpersonas.com`/`api.yourpersonas.com` — same-site despite being cross-origin) per this doc's own Section 4.1 base URL convention; flagged in code comments as needing revisiting to `None; Secure` if that assumption ever changes.
- **Cookie fixes**: `session_id` (`CreateGuestSession`) — `Secure` was hardcoded `true` (broke local HTTP dev entirely; browsers silently drop `Secure` cookies over plain HTTP), now driven by new `isProduction bool` (threaded via new `IsProduction`/`AllowedOrigins` typed Wire aliases, `APP_ENV=="production"`). `SameSite` changed `Lax`→`Strict` per the ticket's explicit ask — safe because this cookie is only used for the guest's own same-site actions; shared result links use `result_id` in the URL (FR-D8), never this cookie.
- `refresh_token` was confirmed to be body-based only, never a cookie anywhere in this codebase — no second cookie needed touching.

#### [C] Route rename: `/v1/dashboard` → `/v1/user-dashboard` (disambiguation, pre-consumer so non-breaking)
- Owner decision (2026-07-15): the word "dashboard" now has two officially distinct meanings — these endpoints are the **user (Member) dashboard** (Epic F), while the sibling repo `dashboard/` is a future **admin/internal dashboard** (V2). Path renamed so the distinction reads straight from the URL; safe now because no client consumes the API yet (FE not integrated) — after FE integration this would have required a `/v2`.
- Changed: `router.go` group, Swagger annotations/tags in `dashboard_handler.go` (`docs/` regenerated), Tech Doc Section 5.4/4.3, PRD Epic F notes, MEMORY.md entry. Go package/handler names (`dashboard`, `DashboardHandler`) intentionally unchanged — only the public contract needed the disambiguation.

---

## [UNRELEASED] — 2026-07-14

### Background workers — PDF generation, Guest TTL purge, Answer/PromptAuditLog scrub (TICKET-06, TICKET-08, TICKET-09)

#### [A] `generate:pdf` worker — real consumer, real producer (TICKET-06)
- New `internal/application/pdf.GeneratePDFUseCase`: fetches the TestResult, marks `pdf_status=processing`, resolves the owner's display name (Member via `UserRepository.FindByID` or Guest via `GuestSessionRepository.FindBySessionID`), collects essay-only answers for direct quoting (FR-E3), matches `TraitScores` against `InsightTemplateRepository.FindMatchingTemplates` for "Strengths & Blind Spots" (FR-E2), renders via a new `PDFRenderer` port, uploads via a new `PDFUploader` port, then marks `pdf_status=completed` with the stored URL.
- New `internal/infrastructure/pdf.MarotoRenderer` — first PDF-rendering code in the repo, backed by `github.com/johnfercher/maroto/v2` (pinned to **v2.3.3**, not latest `v2.4.0`, deliberately — v2.4.0+ requires Go ≥1.26.1 and would have silently bumped this module's `go` directive as a side effect of an unrelated dependency; v2.3.3 needs only Go ≥1.21.1, so `go.mod`'s `go 1.25.0` is untouched).
- **Known gap this ticket does not fix, and does not fake**: `MBTIType`/`GritScore`/`TraitScores` are still zero-value (no scoring-algorithm ticket exists — see TICKET-03's entry below). The renderer is data-driven: MBTI/chart/insight sections show a locale-aware placeholder today and will render real content automatically the moment scoring lands, with zero rework.
- New `s3.Client.Upload(ctx, key, data, contentType) (url string, err error)` — no upload method existed before (only `DeleteByURL`/`PresignedGetURL`). Returns a URL built from `minio-go`'s own `Client.EndpointURL()`, so it round-trips through the existing `keyFromURL` unchanged — no new stored-URL convention introduced.
- New `internal/interfaces/worker/pdf_handler.go` (`PDFHandler.ProcessTask`), registered in `cmd/worker/main.go`'s mux for `taskqueue.TaskGeneratePDF` (event-triggered, no scheduler entry — enqueued on submit, not cron).
- **`pdf_status=failed` transition** wired via `asynq.Config.ErrorHandler` (checks `asynq.GetRetryCount(ctx) >= asynq.GetMaxRetry(ctx)`) rather than inside the use case itself — Asynq calls the handler again on every retry, so only the final-failure hook should flip the status, matching "failed saat max-retry Asynq habis" (TECHNICAL_DOCUMENTATION Section 6). Reusable for any future task type needing the same signal.
- **Producer side fixed too, not just the consumer**: `cmd/api/wire.go` previously bound `assessment.PDFQueueService` to `stubs.NewStubPDFQueueService` (a no-op) — submitted assessments never actually enqueued a `generate:pdf` job. New `internal/infrastructure/queue/asynq.PDFQueueService` adapts the already-implemented `taskqueue.Dispatcher.EnqueuePDFGeneration` to the interface; `internal/infrastructure/stubs/` (now fully unreferenced) removed.

#### [A] Guest TTL purge job — `purge:guest-ttl`, daily cron (TICKET-08)
- New `internal/application/guestpurge.PurgeGuestTTLUseCase`: for every `TestResultRepository.FindExpiredGuestResults` row, deletes the R2/MinIO PDF object **first** (if present), then one `db.Transaction` deleting `Answer`/`PromptAuditLog`/`TestResult` rows, then — after the full sweep — deletes each now-orphaned `GuestSession` once. Per-row failures are logged and skipped rather than aborting the batch: PRD Section 9.6 explicitly designs this job to be idempotent and safe to partially fail (re-deleting an already-gone R2 object is a no-op success, not an error), so tomorrow's tick self-heals.
- New repository methods: `TestResultRepository.DeleteByID`, `AnswerRepository.DeleteByTestResultID` (mirrors the already-existing `PromptAuditLogRepository.DeleteByTestResultID`).
- New `internal/interfaces/worker/purge_handler.go`, registered for `taskqueue.TaskPurgeGuest` (constant already existed, unused until now) with a daily `asynq.Scheduler` entry + boot-kick enqueue, mirroring the existing `deletion:scan-expired` pattern exactly.
- Scope note: only guest sessions tied to an *expired test result* are purged (per the ticket's literal ordering). Orphaned unclaimed guest sessions with zero test results (`GuestSessionRepository.FindExpiredUnclaimed` exists but isn't called by any ticket) are a separate, undefined case — not touched here.

#### [A] Anonymize worker now scrubs `ANSWER` essays + `PROMPT_AUDIT_LOG` (TICKET-09)
- Closes the exact gap `TECHNICAL_DOCUMENTATION.md`'s own v4 changelog note flagged: "Scrub ANSWER esai + PROMPT_AUDIT_LOG menyusul begitu tabelnya ada." Both tables now exist; two calls added inside `AnonymizeUseCase.Anonymize`'s existing `db.Transaction`, alongside the User/GuestSession/TestResult scrub calls already there — same automatic-rollback-on-failure guarantee, no new transaction.
- New `AnswerRepository.ScrubEssayAnswersByUser` (blanks `value` for essay-type answers only, via a `test_results.user_id` + `questions.type='essay_prompt'` subquery join — SJT/Likert answers hold no PII and are left intact) and `PromptAuditLogRepository.DeleteByUserID` (hard-deletes via a `test_results.user_id` subquery join — neither table has its own `user_id` column).
- Deliberately **no `wire.go` changes**, despite the ticket text suggesting constructor injection: the existing transaction block already constructs every tx-scoped repo inline via `pg*.NewXRepository(tx, uc.log)` rather than through injected params, so this follows that established convention instead of growing the constructor's parameter list.

### Result, Question Bank & Member Dashboard endpoints (TICKET-04, TICKET-07)

#### [A] `ResultHandler` — `/v1/questions`, `/v1/results/:id` and sub-routes (TICKET-04)
- `GET /v1/questions` — locale-translated question bank via `content.QuestionRepository.FindAllWithTranslation` (already existed, just never had a caller); new narrow `assessment.QuestionCatalogUseCase`/`QuestionCatalogRepository`.
- `GET /v1/results/:id` — any caller holding the (unguessable UUID) ID gets the full result; this is the mechanism behind FR-D8 "read-only, shareable-without-login link", the same pattern 16Personalities/Buzzfeed-style result pages use. Response includes a new `is_owner` field so the frontend can decide whether to render owner-only affordances (mascot picker, claim/upsell banners) — **this is a judgment call**: the tech-doc route table says "teaser kalau bukan pemilik", but the ticket's own task list and acceptance criteria never define what a teaser omits, and FR-D3/FR-D4 ("blurred insight"/"partial radar chart") read as frontend rendering effects, not backend truncation. Flagged for confirmation rather than guessing at a privacy-relevant field-omission contract with no spec.
- `PATCH /v1/results/:id/mascot-style`, `GET /v1/results/:id/pdf-status`, `GET /v1/results/:id/pdf` — all owner-gated (`FORBIDDEN` 403 otherwise), matching "harus match kepemilikan": Member → `user_id` == JWT subject, Guest → `guest_session_id` == `session_id` cookie value. Shared via one `ResultUseCase.findOwned` guard instead of repeating the check three times.
- `GET /v1/results/:id/pdf` redirects (302) to a signed URL rather than returning JSON — matches the doc's "Redirect ke signed URL R2". New `s3.Client.PresignedGetURL` (minio `PresignedGetObject`, reuses the existing `keyFromURL` helper); new `assessment.PDFSignerService` interface, wired to `*s3.Client` in `cmd/api` (previously only the worker constructed an S3 client — the API process didn't need one until now).
- New `internal/interfaces/http/middleware/security_headers.go` — `NoIndex` middleware sets `X-Robots-Tag: noindex, nofollow` (FR-D9), applied once at the `/v1/results` route-group level so it's structurally impossible for a new sub-route to forget it, instead of repeating the header write in every handler.
- New `AuthMiddleware.OptionalAuth` — resolves Member identity from a Bearer token if present but never rejects the request (unlike `RequireAuth`), for the "Auth: Optional" (Guest-or-Member) routes: `/assessment/submit`, `/results/*`. Extracted a shared `BearerToken(c)` helper and an internal `authenticate()` step so `RequireAuth`/`OptionalAuth` (and `LocaleMiddleware`'s existing best-effort JWT parse) don't each re-implement "trim the Bearer prefix, parse, look up the user".
- **Bug fix in `AssessmentHandler.Submit`**: identity resolution previously checked the `session_id` Guest cookie *first* and only tried the Bearer token in the unreachable `else` branch (the `// TODO: Implement JWT Extraction` the ticket asked to close). A logged-in Member who still carries a stale Guest cookie from before registering would have been silently misfiled as a Guest. Now the Bearer token (via `OptionalAuth`) always takes priority.
- `TestResultRepository.FindHistoryByUser(ctx, userID, page, limit) ([]TestResult, int64, error)` added to the domain interface (shared with TICKET-07 below) — didn't exist; nothing paginated by owner before this.

#### [A] `DashboardHandler` + `DashboardUseCase` — `/v1/dashboard`, `/v1/dashboard/history` (TICKET-07)
- `GET /v1/dashboard` — quota remaining computed on-the-fly (`application.MemberMonthlyQuota - CountMonthlyUsage`, never a stored counter, per FR-F2) plus a `grit_trend` array (last 5 results, oldest-first) for FR-F3.
- `GET /v1/dashboard/history?page=&limit=` — paginated via the new `FindHistoryByUser`; response `meta` carries a reusable `PaginationMeta{page,limit,total,total_pages}` shape.
- New shared `application.MemberMonthlyQuota = 3` constant (`internal/application/constants.go`) — previously duplicated as an unexported `monthlyQuotaLimit` literal only inside the submit usecase; the dashboard needed the same number and duplicating it risked the two drifting apart.
- **Known gap, inherited not introduced**: `GritScore`/`MBTIType`/`TraitScores` are still zero-valued (no scoring-algorithm ticket exists yet — see TICKET-03's changelog entry above). `grit_trend`/`latest_mbti_type` are wired and will populate correctly the moment that gap is closed; they are not faked in the meantime.

#### [C] Router — new route groups, no existing route's behavior changed
- `authMiddleware.OptionalAuth` added to `POST /v1/assessment/submit`; new `/v1/questions`, `/v1/results/*` (group-wrapped with `NoIndex` + `OptionalAuth`), `/v1/dashboard/*` (wrapped with `RequireAuth`, Member-only per FR-F1-F5) route groups in `router.go`. `SetupRouter` gained two constructor params (`ResultHandler`, `DashboardHandler`); `cmd/api/wire.go`/`wire_gen.go` regenerated, `cmd/api/main.go` gained `S3_*` env var loading (mirrors the existing pattern already used by `cmd/worker/main.go`) since the API process now also needs an S3 client for signed PDF URLs.

### Domain layer reorganization — assessment aggregate vs. content catalog

#### [C] `internal/domain/testresult` absorbs `answer` and `promptauditlog`
- `TestResult`, `Answer`, and `PromptAuditLog` merged into one domain package (files: `test_result_entity.go`, `answer_entity.go`, `promptauditlog_entity.go`, `testresult_repository.go`) — mirrors the `account_repository.go` pattern (multiple interfaces, one file), applied here on real evidence: all three are always mutated inside the same `db.Transaction` in `SubmitAssessmentUseCase.Execute` (TICKET-03), and `Answer`/`PromptAuditLog` both FK into `TestResultID`. `TestResult` is the aggregate root.
- `testresult.Repository` renamed to `testresult.TestResultRepository` (interface now needs an explicit prefix since the package holds three repositories, not one — matches `account`'s `UserRepository`/`GuestSessionRepository`/etc. naming)
- `answer.Repository` → `testresult.AnswerRepository`; `promptauditlog.Repository` → `testresult.PromptAuditLogRepository`
- Persistence layer (`internal/infrastructure/persistence/postgres/assessment/`) already lived together from the TICKET-01 review pass and an earlier manual move — this makes the domain layer match that same evidence-based grouping

#### [A] `internal/domain/content` — new package for `question`+`questiontranslation`+`insighttemplate`
- Deliberately kept **separate** from `testresult` despite living in the same infra directory: `Question`/`InsightTemplate` are locale-aware, read-only reference/catalog data — seeded once (TICKET-10), read by many `TestResult`s, never mutated inside the submit transaction. Merging them into `testresult` would have been "feels quiz-related," not evidence of shared mutation/dependency — the same anti-pattern this whole reorganization is meant to avoid.
- Real evidence *for* this specific merge: both already share the exact same fallback-resolution algorithm (`pkg/locale.PickWithFallback`), are seeded together in one ticket/binary (`cmd/seed/main.go`), and are both consumed as pure lookup tables.
- Deduplicated `QuestionTranslation`: it existed as two byte-for-byte identical struct definitions (`question.QuestionTranslation` and `questiontranslation.QuestionTranslation`) — now a single `content.QuestionTranslation`. Two identical mapper functions in the persistence layer collapsed into one as a result.
- `question.Repository` → `content.QuestionRepository`, `questiontranslation.Repository` → `content.QuestionTranslationRepository`, `insighttemplate.Repository` → `content.InsightTemplateRepository`

### Assessment — Submit Usecase & AI Output Validation (TICKET-03, TICKET-05)

#### [A] `SubmitAssessmentUseCase.Execute` fully implemented — was a skeleton of placeholder comments
- Idempotency check (SHA-256 hash of the answer set, order-independent) → distributed lock (`quota_lock:{session_id}`, held for the full submission, TTL 20s > Gemini's own 15s timeout) → monthly quota check (Members only, `>= 3` rejected) → per-essay 4000-char cap + crisis-keyword wellbeing scan → Gemini call (skipped entirely if there are no essay answers, rather than spending a call on empty input) → single `db.Transaction` for TestResult + Answers + PromptAuditLog + referral event → best-effort PDF enqueue → idempotency cache save (TTL 24h)
- New sentinel `application.ErrQuotaExceeded`; reuses `ErrLockNotAcquired`/`ErrIdempotencyKeyReused` from TICKET-02
- Referral event (FR-G1): fires `test_completed` only for a Member's first-ever completed/fallback_static result — new `TestResultRepository.CountCompletedByUser` (all-time, distinct from the existing month-scoped `CountMonthlyUsage`)
- New `QuestionRepository.FindByIDs` (bulk, single query) to classify which answers are essays without an N+1 lookup per answer
- **Known gap, not invented**: `MBTIType`/`GritScore`/`TraitScores` are left at zero-value — no ticket in the roadmap defines the psychometric scoring algorithm yet. Flagged in-code and here rather than guessing a formula.
- **Fixed pre-existing bug found while wiring this up**: `TestResultRepository.toModel` marshaled a nil `TraitScores` map into an empty Go string, but the column is `jsonb` — Postgres rejects `''` as invalid JSON. Would have broken the very first real insert into this table. Now defaults to `"{}"`.
- **Architecture fix**: removed `redis.QuotaLockKey` (added in the TICKET-02 pass) — having the application layer import an infra package just to format a string violated the dependency-inversion rule that layer's own local interfaces exist to enforce. The key is now formatted inline in the usecase.

#### [A] `pkg/aivalidator.ValidateOutput` — Gemini output sanity check (TICKET-05)
- Rejects output under 20 chars, and known refusal phrases (locale-specific list + English always checked as a safety net, since a refusal can come back in English regardless of requested locale)
- **Fixed a real false-positive bug in the pre-existing scaffold**: the original patterns included bare `"i'm sorry"` and `"maaf"` — both are common words that would trip on legitimate empathetic analysis text (e.g. "I'm sorry to hear about the challenges you faced..."). Replaced with more specific multi-word phrases; verified against both refusal and legitimate-empathetic-text cases.
- `gemini.Client.GenerateSummary` signature changed to also return the exact `rawPrompt` sent (system_instruction + essay text), even on error — so `PROMPT_AUDIT_LOG.raw_prompt` reflects what was *actually* sent rather than a second, potentially-drifted reconstruction of it. Audit log entry (with `FlaggedAnomaly`) is written inside the same transaction as the TestResult it references, for every call that was actually made (skipped when there was no essay content to send).

### Platform Services — Redis Lock/Idempotency, Locale Negotiation, HIBP (TICKET-02, TICKET-15, TICKET-16)

#### [A] `internal/infrastructure/cache/redis/lock.go` — real `DistributedLockService` (TICKET-02)
- SETNX-based acquire with a per-acquisition owner token (`uuid`); `ReleaseLock` does a compare-and-delete via `redis.NewScript` Lua CAS instead of a blind `DEL` — prevents request A from deleting request B's lock if A's TTL already expired while A was still processing. First Lua script usage in this codebase, added specifically to close this race rather than ship the naive version.
- `QuotaLockKey(id string) string` exported so `quota_lock:{id}` isn't hand-formatted at each call site (TICKET-03 will consume this)

#### [A] `internal/infrastructure/cache/redis/idempotency.go` — real `IdempotencyService` (TICKET-02)
- `Check`/`Save` against Redis, JSON envelope `{payload_hash, response}`, TTL 24h
- Hash mismatch on a reused idempotency key returns new sentinel `application.ErrIdempotencyKeyReused` (anti cache-poisoning, per ticket spec) instead of silently serving someone else's cached response

#### [A] `internal/interfaces/http/middleware/locale.go` — `LocaleMiddleware` (TICKET-15)
- Negotiation order per FR-I1: `?locale=` query → authenticated member's `preferred_locale` (best-effort JWT parse + `userRepo.FindByID`, never blocks the request on failure, unlike `AuthMiddleware.RequireAuth`) → `Accept-Language` header → `locale` cookie → `en` default
- Registered globally in `router.go`; `middleware.LocaleFromContext(c)` mirrors the existing `UserIDFromContext(c)` pattern
- `pkg/locale` gained `IsSupported(code string) bool` and `ParseAcceptLanguage(header string) string` — pure, framework-independent functions so the negotiation primitives are reusable outside HTTP (and unit-testable without Echo)
- Not wired into `assessment_handler.go`'s Submit flow yet — that's TICKET-03/04 scope, left untouched deliberately

#### [A] `internal/infrastructure/security/hibp.go` — real `HIBPBreachChecker` (TICKET-16)
- k-Anonymity: SHA-1 → first 5 hex chars sent to `api.pwnedpasswords.com/range/{prefix}`, response stream-scanned line-by-line for the matching suffix — full password/hash never leaves the process
- `Add-Padding: true` request header (HIBP-recommended) so response size can't leak whether a match was found
- Own 5s timeout independent of the caller's `ctx` cancellation (registration shouldn't hang on a slow third party) but does NOT use `context.WithoutCancel` like the Gemini client — no token cost/in-flight work to protect here, so honoring caller cancellation is correct
- Fails open on any transport/parse error (matches the existing `ValidateNewPassword` behavior and the Gemini fallback precedent), but now logs a `Warn` on that path — previously silent
- Replaces `auth.NewNoopBreachChecker` in `cmd/api/wire.go`; `NoopBreachChecker` itself kept in `account.go` as a test double, not deleted

### Assessment — Persistence Layer & DB Seeder (TICKET-01, TICKET-10)

#### [A] GORM models + AutoMigrate untuk 5 tabel assessment (commit `e3fc758`)
- `QuestionModel`, `QuestionTranslationModel` (unique `(question_id, locale)`), `AnswerModel` (unique `(test_result_id, question_id)` — FR-B10 upsert revisi jawaban), `InsightTemplateModel` (unique `(insight_key, locale)`), `PromptAuditLogModel` (index `expires_at` untuk purge TTL 30 hari) — semua didaftarkan di `cmd/migrate/main.go`
- `internal/infrastructure/persistence/postgres/assessment/` — repository Postgres nyata; `StubTestResultRepository` & `StubAnswerRepository` dicabut dari `cmd/api/wire.go` (lock/idempotency/PDF-queue masih stub → TICKET-02/06)

#### [A] `cmd/seed/main.go` — seeder idempotent question bank & insight templates (commit `e3fc758`)
- 3 SJT + 10 Likert (2 reverse-scored, 1 attention check — rasio FR-B2) + 2 essay prompt, terjemahan EN+ID lengkap, upsert `ON CONFLICT DO UPDATE` (aman dijalankan berulang)
- 2 insight template (`grit_increase`, `grit_high_threshold`) × 2 locale

#### [C] Rapikan struktur repository assessment (follow-up review)
- `AnswerRepository` yang tadinya merangkap 3 domain dipecah: `InsightTemplateRepository` (`insighttemplate_persistence.go`) & `PromptAuditLogRepository` (`promptauditlog_persistence.go`) jadi struct terpisah sesuai TICKET-01 — penting sebelum TICKET-09 nambahin `ScrubEssayAnswersByUser`/`DeleteByUserID`
- Logika fallback locale (`locale target → en`) yang terduplikasi di `FindAllWithTranslation` dan `FindMatchingTemplates` disatukan jadi `pkg/locale.PickWithFallback` generic — sesuai DRY rule `AGENTS.md` (FR-I9, satu resolver untuk QUESTION_TRANSLATION dan INSIGHT_TEMPLATE)

### Auth — Session & Password Lifecycle Completion

#### [A] 6 endpoint Auth baru (FR-H4, FR-H6, FR-H8, NFR JWT)
- `POST /v1/auth/refresh` — refresh token **rotation**: token lama langsung didenylist di Redis begitu ditukar, plus `token_version` check
- `POST /v1/auth/logout` (Auth: Required) — revoke SATU sesi lewat denylist `refresh_token`; access token yang lagi dipakai tetap valid sampai expired alami
- `POST /v1/auth/logout-all` (Auth: Required) — revoke SEMUA sesi lewat increment `USER.token_version`, tanpa body
- `POST /v1/auth/forgot-password`, `POST /v1/auth/verify-reset-otp`, `POST /v1/auth/reset-password` — 3-step password reset; `reset_token` JWT umur pendek (~15 menit) dikonsumsi atomic via Redis `GETDEL` (anti-replay/TOCTOU)

#### [A] POST /v1/auth/change-password (FR-H9, baru — bukan bagian rencana awal)
- Ganti password saat sudah login pakai `old_password` + `new_password` + `retry_new_password`, tanpa perlu OTP
- Sukses → increment `USER.token_version` (revoke semua device LAIN) + auto re-issue `access_token`/`refresh_token` baru untuk device yang sedang dipakai, supaya user tidak ikut ter-logout dari aksinya sendiri

#### [A] Infrastruktur pendukung endpoint di atas
- `internal/infrastructure/cache/redis/token_store.go` — `TokenStore`: `StoreResetJTI`/`ConsumeResetJTI` (GETDEL, single-use), `DenylistRefreshJTI`/`IsRefreshJTIDenylisted`
- `internal/infrastructure/cache/redis/ip_rate_limit.go` — `IPRateLimitService` per-IP terpisah dari account lockout (FR-H6): login 20x/15 menit, register 10x/15 menit
- `internal/interfaces/http/middleware/auth.go` — `AuthMiddleware.RequireAuth`: validasi Bearer token + cek `token_version` vs DB (`TOKEN_VERSION_MISMATCH`); menerima header dengan ATAU tanpa prefix `Bearer` (kemudahan testing, mis. Swagger Authorize)
- `@securitydefinitions.apikey BearerAuth` di `cmd/api/main.go` — tombol **Authorize** kini muncul di Swagger UI untuk semua endpoint ber-`@Security BearerAuth`

#### [C] Konsolidasi `application/auth` dari 12+ file jadi 4 file per keluarga dependency
- `register.go`, `session.go` (login/refresh/logout/logout-all/change-password/reset-password — semua yang pegang `jwtService`/`tokenStore`), `account.go` (OTP verify/resend, forgot-password, shared password policy — semua yang pegang OTP rate limiter), `create_guest_session.go`
- `internal/domain/account/` — gabungan `user`+`guestsession`+`verificationtoken`+`referral` jadi satu bounded context; 4 repository interface dikonsolidasi ke satu `account_repository.go` (entity tetap 1 file per aggregate)
- `internal/infrastructure/persistence/postgres/account/` — 1 file per aggregate, suffix diganti dari `repository.go` jadi `_persistence.go` (mis. `user_persistence.go`)
- `pkg/repository.BaseRepository[T]` (generic) tidak lagi wajib dipakai semua domain — dokumentasi `AGENTS.md`/`TECHNICAL_DOCUMENTATION.md` diperbarui sesuai realita (bentuk repository konkret mayoritas custom query, gak match pola CRUD generic)

#### [F] Bug fixes dari live testing
- `referral_code` di `/auth/register` sekarang menerima `""` (string kosong) selain `null`/omitted sebagai "tidak ada kode" — FE kadang kirim `""` alih-alih `null` untuk field yang di-clear
- Worker `send:email` sebelumnya dummy handler yang cuma log tanpa benar-benar kirim email — diganti `internal/interfaces/worker/email_handler.go` yang memanggil `mailer.SendOTP` sungguhan, di-wire di `cmd/worker/main.go`
- `docker/docker-compose.dev.yml` — service `worker` tidak override `SMTP_HOST`, sehingga fallback ke `.env`'s `SMTP_HOST=localhost` yang di dalam container merujuk ke dirinya sendiri, bukan container `mailpit` (`dial tcp [::1]:1025: connection refused`). Fix: `environment: SMTP_HOST=mailpit`

### Account, Referral, Deletion Request & Operational Endpoints

#### [A] 4 endpoint baru di bawah `/v1/account/*` (FR-I3, FR-F1, FR-G1, FR-G2, FR-G2a, FR-G3)
- `PATCH /v1/account/profile` (Auth: Required) — partial update `display_name`/`age`/`status`/`preferred_locale`
- `GET /v1/account/referral-code` (Auth: Required) — ambil kode referral existing, atau generate baru (8 karakter, charset tanpa huruf/angka ambigu `0/O`/`1/I/L`) kalau belum ada, dengan retry maksimal 5x kalau collision
- `POST /v1/account/delete-request` (Auth: Required) — mulai grace period 14 hari, tolak `DELETION_ALREADY_REQUESTED` (409) kalau sudah ada request aktif
- `POST /v1/account/delete-request/cancel` (Auth: Required) — batalkan selama masih `pending_grace`; `NO_ACTIVE_DELETION_REQUEST` (404) / `DELETION_ALREADY_PROCESSING` (409) kalau tidak bisa
- **Belum termasuk**: worker `anonymize:user` (scrub PII + hapus PDF R2) dan cron pemicunya — di luar scope endpoint request/cancel ini

#### [A] GET /healthz (Operational)
- Cek `Postgres` + `Redis` saja — MinIO/R2 dan Mailpit/SMTP sengaja dikecualikan (dipakai async lewat worker atau cuma nyentuh subset endpoint PDF, jadi gak boleh bikin load balancer narik instance API sehat keluar dari rotasi)
- Response `{ "database": "up|down", "redis": "up|down" }`, HTTP 503 kalau salah satu down

#### [C] `POST /v1/auth/verify-email-otp` sekarang auto-login
- Response sekarang balikin `access_token`+`refresh_token` — user sudah membuktikan tau password (saat register) + punya akses email (OTP ini), jadi `/auth/login` terpisah setelahnya jadi friksi yang gak perlu
- Method `VerifyEmailOTP` dipindah dari `account.go` ke `session.go` (butuh `jwtService`, satu keluarga dengan `Login`/`RefreshToken`/`ResetPassword`)

#### [A] Package baru: `application/profile`, `application/deletionrequest`, `infrastructure/persistence/postgres/deletionrequest`
- Domain `deletionrequest` (entity + repository interface) sudah ada dari scaffolding awal, reuse penuh — cuma persistence layer yang baru
- Sengaja TIDAK digabung ke `application/auth`/`domain/account` — `ProfileUseCase`/`DeletionUseCase` gak share dependency set dengan use case manapun di situ, dan `DataDeletionRequest` gak pernah dimutasi dalam transaksi bareng `USER` (beda dengan GuestSession/VerificationToken/ReferralCode yang wajib satu transaksi saat Register). Detail alasan di PRD Section 9

#### [C] Handler HTTP dikonsolidasi jadi satu `AccountHandler`
- `profile_handler.go` + `deletion_handler.go` digabung jadi `account_handler.go` — satu handler menaungi SEMUA route `/account/*`, meniru pola `AuthHandler` yang sudah menaungi semua `/auth/*` sejak awal (grouping per URL namespace di HTTP layer, beda prinsip dari grouping per shared-dependency di application layer)

#### [F] Bug fixes
- `register.go` `buildUser` — data profil (`display_name`/`age`/`status`) dari `GuestSession` sekarang cuma di-copy kalau `!guest.IsClaimed()`. Sebelumnya: kalau cookie `session_id` lama (yang guest session-nya SUDAH diklaim akun lain) kepakai ulang buat register akun baru, data profil guest lama itu tetap bocor ke akun baru walau kepemilikannya gak ikut ke-transfer
- `infrastructure/persistence/postgres/db.go` — GORM logger di-set `IgnoreRecordNotFoundError: true`. Sebelumnya setiap lookup yang wajar-gak-ketemu (email belum terdaftar, referral code belum digenerate, dst — semua ditangani sebagai `nil, nil` di kode) tetap tercetak sebagai baris log "record not found", padahal bukan error sungguhan

### Anonymization Worker — Deletion Pipeline End-to-End (FR-G2, PRD Section 9.3)

#### [A] Worker `anonymize:user` + scan terjadwal `deletion:scan-expired`
- `application/deletionrequest/anonymize.go` — `AnonymizeUseCase`:
  - **Scan side** (`ProcessExpired`): ambil semua request `pending_grace` yang lewat 14 hari via `FindExpiredGracePeriod`, enqueue job `anonymize:user` per user (queue `low`), lalu CAS status → `processing`. Urutan **enqueue dulu baru flip status** disengaja: kalau enqueue gagal, row tetap `pending_grace` dan scan berikutnya retry sendiri (urutan kebalikan bisa ninggalin row nyangkut di `processing` tanpa job)
  - **Worker side** (`Anonymize`): guard status (`cancelled` → drop, `completed` → skip, idempotent) → hapus SEMUA objek PDF R2 milik user SEBELUM kolomnya di-null-kan (aturan AGENTS.md) → satu `db.Transaction`: scrub `USER` (email→`deleted-{id}@anonymized.invalid`, display_name/age/status/password_hash dikosongkan, `deleted_at`+`anonymized_at` diisi, `token_version++`), scrub `GUEST_SESSION` yang diklaim user (display_name/age/status/ip_hash), null `TEST_RESULT.ai_summary_text`+`pdf_url` (agregat mbti/grit dipertahankan) → status `completed` → enqueue email `deletion_confirmed` ke `notification_email` snapshot
- `internal/interfaces/worker/anonymize_handler.go` — delivery layer untuk kedua task type
- Scheduler `asynq.Scheduler` di `cmd/worker`: scan tiap 1 jam + satu scan langsung saat boot (biar restart deploy gak bikin request expired nunggu 1 jam lagi)
- **Belum termasuk** (tabelnya sendiri belum ada — assessment epic masih jalan di stub): scrub `ANSWER` esai + `PROMPT_AUDIT_LOG`. Begitu tabel itu dibuat, tambahkan scrub call di transaksi `Anonymize`

#### [A] Object storage client (MinIO dev / Cloudflare R2 prod)
- `internal/infrastructure/storage/s3/client.go` — wrapper `minio-go/v7`, satu client untuk dua environment (sama-sama S3 API). Method `DeleteByURL` idempotent (delete key yang sudah hilang = sukses, pas untuk retry Asynq). Dependency baru: `github.com/minio/minio-go/v7`
- Port-nya (`PDFStorage`) didefinisikan di application layer — dependency rule tetap bersih

#### [A] Email `deletion_confirmed`
- `Mailer.SendDeletionConfirmed` (en/id) + case baru di `email_handler.go` — dikirim SETELAH scrub, ke `DATA_DELETION_REQUEST.notification_email` (snapshot), bukan `USER.email` yang sudah teranonimkan

#### [C] `cmd/worker` — sekarang konek Postgres + MinIO/R2
- Sebelumnya worker cuma butuh Redis+SMTP. Manual DI untuk: GORM DB, S3 client, asynq.Client+Dispatcher (worker sekarang juga PRODUSEN task: scan → fan-out anonymize, anonymize → email konfirmasi)
- Graceful shutdown diperluas: `scheduler.Shutdown()` dulu (stop tick baru) baru `srv.Shutdown()` (tunggu job in-flight)
- `docker-compose.yml` worker: `DB_HOST=postgres`; `docker-compose.dev.yml` worker: `S3_ENDPOINT=http://minio:9000` + `depends_on: minio` (pola bug yang sama dengan `SMTP_HOST=mailpit` sebelumnya)

#### [A] Method repository baru (dipakai pipeline di atas)
- `account.UserRepository.Anonymize` — satu UPDATE `Unscoped` (retry tetap kena row yang sudah soft-deleted)
- `account.GuestSessionRepository.AnonymizeClaimedByUser`
- `testresult.Repository.FindPDFURLsByUser` + `ScrubPersonalDataByUser`
- `deletionrequest.Repository.FindByID` + `TransitionStatus` (compare-and-swap status, return `rowsAffected > 0`)

#### [F] Race condition fixes (dua-duanya sempat di-flag sebagai known issue)
- **`POST /account/delete-request` double-submit**: partial unique index baru `uniq_active_deletion_per_user` di `data_deletion_requests(user_id) WHERE status pending_grace/processing` — check-then-insert yang kalah race sekarang kena unique violation dan di-map ke `DELETION_ALREADY_REQUESTED` (409), bukan bikin row `pending_grace` ganda yang gak bisa di-cancel. **Butuh `go run ./cmd/migrate`** untuk membuat index-nya
- **`POST /account/delete-request/cancel` vs scan grace-period**: cancel sekarang CAS (`pending_grace`→`cancelled`), bukan blind update — gak bisa lagi "membatalkan" request yang job anonymize-nya sudah in-flight
- **`GET /account/referral-code` double-request**: unique violation saat insert sekarang dibedakan — kalau kode user ini sudah dibuat request paralel, balikin kode itu (bukan 500); kalau collision kode antar-user, retry generate
- `db.go` — `TranslateError: true` di GORM config, supaya use case bisa cek `errors.Is(err, gorm.ErrDuplicatedKey)` tanpa string-matching SQLSTATE

---

## [UNRELEASED] — 2026-07-07

### Scaffolding & Project Foundation

#### [A] Project Structure & Configuration
- Initialize Go module `github.com/aprxty3/your_persona_controller.git` (Go 1.25)
- `go.mod` — main dependencies: Echo v4, GORM, google/wire, Asynq, go-redis/v9, google/genai (Gemini), google/uuid
- `.air.toml` & `.air.worker.toml` — hot-reload configuration for `cmd/api` and `cmd/worker`
- `.golangci.yml` — linting rules
- `Makefile` — targets: `wire`, `migrate`, `run-api`, `run-worker`, `test`, `lint`, `tidy`, `swag`
- `.env.example` — all env vars documented by category (Server, DB, Redis, S3, SMTP, Gemini, Auth, Turnstile, i18n)

#### [A] cmd/api/main.go
- Graceful shutdown pattern: `SIGTERM`/`SIGINT` -> `http.Server.Shutdown(ctx)` with timeout from `SHUTDOWN_TIMEOUT` env (default 30s), according to **PRD Section 9.5**
- Placeholder `InitializeAPI` (commented) ready to be uncommented once `make wire` succeeds

#### [A] cmd/api/wire.go
- Skeleton Wire injector `InitializeAPI(geminiAPIKey, geminiModel, maxConcurrent)` with TODO markers for DB, Redis, Asynq providers

---

### Domain Layer — Entities & Repository Interfaces

> All domain entities follow Clean Architecture + DDD rules: no imports of GORM, HTTP SDK, or any external dependency. Pure business rules.

#### [A] internal/domain/user/entity.go
- Struct `User` — complete according to ERD: `ID`, `Email`, `PasswordHash`, `DisplayName`, `Age`, `Status`, `ReferredByCode`, `PreferredLocale`, `EmailVerifiedAt`, `CreatedAt`, `DeletedAt`, `AnonymizedAt`, `TokenVersion`
- Additional fields for auth: `FailedLoginCount`, `LockedUntil` (account-level lockout **FR-H3**)
- Helper methods `IsEmailVerified()` and `IsLocked()`
- Repository interface: `Create`, `FindByID`, `FindByEmail`, `Update`, `IncrementTokenVersion`, `UpdateLoginAttempt`

#### [A] internal/domain/guestsession/entity.go
- Struct `GuestSession` — `SessionID`, `IPHash`, `DisplayName`, `Age`, `Status`, `Locale` (**FR-I2**), `ClaimedByUserID`, `CreatedAt`, `ExpiresAt`
- Helper methods `IsClaimed()` and `IsExpired()`
- Repository interface: `Create`, `FindBySessionID`, `Update`, `FindExpiredUnclaimed`, `DeleteBySessionID`

#### [A] internal/domain/testresult/entity.go
- Typed constants: `ResultStatus` (`processing`, `completed`, `fallback_static`) and `PDFStatus` (`pending`, `processing`, `completed`, `failed`)
- Struct `TestResult` — complete according to ERD including all token usage fields (**FR-C4**) and `WellbeingFlag` (**FR-B11**)
- Documented XOR ownership invariant (`user_id` XOR `guest_session_id`) according to ERD
- Helper method `IsExpired()` — endpoints must treat expired results as 404 (**PRD 9.6**)
- Repository interface: `Create`, `FindByID`, `FindByShareToken`, `Update`, `CountMonthlyUsage` (Asia/Jakarta TZ — **PRD 9.1**), `FindExpiredGuestResults`, `UpdatePDFStatus`

#### [A] internal/domain/answer/entity.go
- Struct `Answer` — `ID`, `TestResultID`, `QuestionID`, `Value`, `CreatedAt`, `UpdatedAt`
- Documented composite UNIQUE constraint `(test_result_id, question_id)` according to ERD
- Repository interface: `UpsertAnswers` (ON CONFLICT DO UPDATE — **FR-B10**), `FindByTestResultID`

#### [A] internal/domain/verificationtoken/entity.go
- Typed constant `TokenType` (`email_verification`, `password_reset`)
- Struct `VerificationToken` — `ID`, `UserID`, `Token`, `Type`, `AttemptCount`, `ExpiresAt`, `UsedAt`, `CreatedAt`
- Critical security note: lookup MUST be scoped to `(user_id, type)` — not by token only (space of 1 million 6-digit OTP combinations is not globally unique)
- Repository interface: `Create`, `FindActiveByUserAndType`, `IncrementAttemptCount`, `MarkUsed`, `ExpireAllActiveForUser` (single-token invariant — **AGENTS.md Security Rules**)

#### [A] internal/domain/referral/entity.go
- Typed constant `ReferralEventType` (`signup`, `test_completed`)
- Struct `ReferralCode` — `ID`, `UserID`, `Code`, `CreatedAt`
- Struct `ReferralEvent` — `ID`, `ReferralCodeID`, `ReferredUserID`, `EventType`, `CreatedAt`
- Privacy UU PDP note: data returned to UI MUST be aggregated/masked (**TECHNICAL_DOCUMENTATION Section 5.5**)
- Repository interface: `CreateCode`, `FindCodeByUserID`, `FindCodeByCode`, `CreateEvent`, `CountEventsByCodeID`

#### [A] internal/domain/deletionrequest/entity.go
- Typed constant `DeletionStatus` (`pending_grace`, `processing`, `completed`, `cancelled`)
- Struct `DataDeletionRequest` — `ID`, `UserID`, `NotificationEmail` (snapshot before anonymization), `Status`, `RequestedAt`, `CompletedAt`
- Repository interface: `Create`, `FindActiveByUserID`, `UpdateStatus`, `FindExpiredGracePeriod`

#### [A] internal/domain/question/entity.go
- Typed constants: `QuestionSection` (`A`, `B`, `C`) and `QuestionType` (`mc`, `likert`, `essay_prompt`)
- Struct `Question` — locale-agnostic metadata: `ID`, `Section`, `Type`, `IsReverseScored`, `IsAttentionCheck`, `DisplayOrder`
- Struct `QuestionTranslation` — locale-specific: `ID`, `QuestionID`, `Locale`, `QuestionText`, `Options`
- Repository interface: `FindAllWithTranslation` (with fallback to `en` — **FR-I9**), `FindByID`

#### [A] internal/domain/questiontranslation/entity.go
- Struct `QuestionTranslation` — standalone for direct lookup per `(question_id, locale)`
- Repository interface: `FindByQuestionAndLocale`, `UpsertTranslation`

#### [A] internal/domain/promptauditlog/entity.go
- Struct `PromptAuditLog` — `ID`, `TestResultID`, `RawPrompt`, `RawResponse`, `FlaggedAnomaly`, `CreatedAt`, `ExpiresAt` (30 days)
- Security note: this table must not be exposed via public API
- Repository interface: `Create`, `DeleteByTestResultID` (anonymization worker), `DeleteExpired` (purge cron)

#### [A] internal/domain/insighttemplate/entity.go *(new folder)*
- Typed constant `ConditionType` (`increase`, `decrease`, `threshold`)
- Struct `InsightTemplate` — `ID`, `InsightKey`, `Locale`, `Trait`, `ConditionType`, `MinDelta`, `ThresholdValue`, `TemplateText`, `IsActive`
- Repository interface: `FindMatchingTemplates` (with fallback to `en` — **FR-I9**)

---

### Infrastructure Layer — Shared Packages

#### [A] pkg/repository/base.go
- Generic `BaseRepository[T any]` with CRUD operations: `Create`, `FindByID`, `Update`, `Delete`
- All 10 Postgres repositories compose from here (DRY — **AGENTS.md DRY Rules**)

#### [A] pkg/httpresponse/response.go
- Standard response envelope: `{ success, data, meta }` for success; `{ success, error: { code, message } }` for failure
- `Success()` and `Error()` functions — called by all handlers, no inline formatting (**TECHNICAL_DOCUMENTATION Section 4.3**)

#### [A] pkg/taskqueue/dispatcher.go *(skeleton)*
- Asynq enqueue wrapper — used by all job types (PDF, email, anonymize)

#### [A] pkg/locale/ *(skeleton)*
- Locale-fallback resolver — one function for `QUESTION_TRANSLATION` and `INSIGHT_TEMPLATE` (**FR-I9**)

#### [A] pkg/aivalidator/ *(skeleton)*
- Gemini output validation — composable & unit-testable (**FR-C5**)

---

### Infrastructure Layer — Gemini Client

#### [A] internal/infrastructure/gemini/client.go
- `Client` struct with semaphore concurrency control (`GEMINI_MAX_CONCURRENT`)
- Implements `assessment.AIGeneratorService` interface
- Context-aware semaphore acquisition (abort if user disconnects during waiting — **AGENTS.md Security Rules**)

---

### Interfaces Layer — HTTP

#### [A] internal/interfaces/http/router.go
- Echo router setup with global middleware: Recover, Logger, BodyLimit 32KB (**Denial of Wallet protection**)
- Registered routes: `POST /v1/assessment/submit`

#### [A] internal/interfaces/http/handler/assessment_handler.go
- Submit handler — extract Idempotency-Key, parse body, extract session/token, delegate to use case
- Consistent use of `httpresponse.Error` and `httpresponse.Success`

---

### Application Layer — Use Cases

#### [A] internal/application/assessment/submit_assessment.go
- `SubmitAssessmentUseCase` struct with dependency interfaces: `TestResultRepository`, `AnswerRepository`, `AIGeneratorService`, `DistributedLockService`, `IdempotencyService`, `PDFQueueService`
- Skeleton `Execute()` — outline of 9 steps according to **TECHNICAL_DOCUMENTATION Section 5.2**
- 2-phase context cancellation: abort if user disconnects during waiting semaphore; `context.WithoutCancel` after in-flight (**AGENTS.md**)
- Fallback to `fallback_static` if Gemini fails (**FR-C2**)

---

### Auth & Session Implementation (Day 1 Priority)

#### [A] GORM Models & Repositories
- Update `UserModel` and `GuestSessionModel` with account lockout counters (`FailedLoginCount`, `LockedUntil`) and locale preferences.
- Add new `VerificationTokenModel` table structure for tracking email OTPs.
- Implement concrete Postgres repositories: `UserRepository`, `GuestSessionRepository`, and `VerificationTokenRepository`.

#### [A] Redis Infrastructure & OTP Rate Limiting
- Add Redis client configuration provider.
- Implement `OTPRateLimitService` with rolling 5x/day cap and 60-second cooldown per email using pipelines.

#### [A] Security & Session Tokens (JWT)
- Implement `JWTService` with custom claims containing the `token_version` to support remote session invalidation.

#### [A] Auth Use Cases
- Implement `CreateGuestSessionUseCase` to support onboarding form submissions for guests.
- Implement `RegisterUseCase` with atomic transaction-based guest-claiming, referral tracking, and asynchronous email OTP dispatching.
- Implement `VerifyEmailOTPUseCase` with daily attempt tracking.
- Implement `ResendEmailOTPUseCase` with rate limit checks and old active token invalidation.
- Implement `LoginUseCase` with account lockout policy (10 failed consecutive attempts -> 15 min lock).

#### [A] Delivery Layer (HTTP Router & Handlers)
- Create `AuthHandler` containing standard envelopes and correct HTTP status mappings (e.g. 423 for Account Lock, 429 for Max OTP attempts).
- Add full Swaggo documentation annotations to all 5 priority auth endpoints.
- Wire routes and cookies setup in `router.go`.
- Compile and generate `wire_gen.go` using Google Wire.
- Generate swagger OpenAPI documentation under `docs/`.

---

### Verification

```
go run github.com/google/wire/cmd/wire ./cmd/api  ✅ OK (generated wire_gen.go)
swag init -g cmd/api/main.go -o docs              ✅ OK (generated docs/)
go build ./cmd/api                                ✅ OK (server compiles successfully)
go build ./cmd/migrate; go build ./cmd/worker     ✅ OK (auxiliary entrypoints compile successfully)
```

---

## Planned (Next Steps)

- [x] ~~Complete GORM models & Postgres repositories for the remaining domains~~ — done 2026-07-13 (TICKET-01: Question, QuestionTranslation, Answer, InsightTemplate, PromptAuditLog + seeder TICKET-10)
- [x] ~~Complete remaining Auth use cases (Forgot Password, Verify Reset OTP, Reset Password, Logout, Logout-all)~~ — done 2026-07-13, see UNRELEASED entry above (plus unplanned `change-password`)
- [x] ~~Implement Account/Referral endpoints (profile update, referral code)~~ — done 2026-07-13
- [x] ~~Implement `anonymize:user` worker (scrub PII, delete R2 PDFs) + cron trigger via `FindExpiredGracePeriod`~~ — done 2026-07-13 (hourly `asynq.Scheduler` scan + startup kick); remaining sub-item (scrub `ANSWER` essay + `PROMPT_AUDIT_LOG`) closed 2026-07-14 (TICKET-09)
- [x] ~~Complete worker implementation (Asynq background workers for PDF + `purge:guest-ttl`)~~ — done 2026-07-14 (TICKET-06, TICKET-08); email OTP & anonymize were already done 2026-07-13
- [x] ~~Implement Redis-backed Distributed Lock & Idempotency Service~~ — done 2026-07-13 (TICKET-02); not yet wired into `SubmitAssessmentUseCase.Execute` itself, that's TICKET-03
- [x] ~~Implement Locale Negotiation Middleware~~ — done 2026-07-13 (TICKET-15); not yet consumed by any handler, that's TICKET-03/04
- [x] ~~Integrate Turnstile verification~~ — done 2026-07-15 (TICKET-11)
- [x] ~~Have I Been Pwned API breach checks~~ — done 2026-07-13 (TICKET-16)
- [x] ~~Email i18n message catalog externalized to JSON~~ — done 2026-07-15 (TICKET-12; locale wiring itself was already complete)
- [x] ~~Database backup script + restore drill docs~~ — done 2026-07-15 (TICKET-13); cron not yet scheduled on a real server (no production VM yet, see be_tickets/ticket-26)
- [x] ~~CSRF & CORS protection~~ — done 2026-07-15 (TICKET-14)
- [x] ~~Add integration testing via testcontainers-go~~ — done 2026-07-16 (TICKET-21; unit tests for every security-critical path listed in Tech Doc Section 10, + Postgres/Redis integration tests behind the `integration` build tag)
- [x] ~~Dashboard micro-insight (FR-F4, rule-based)~~ — done 2026-07-16 (TICKET-20)

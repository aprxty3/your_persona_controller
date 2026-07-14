# CHANGELOG — controller-api

Format: [Semantic Versioning](https://semver.org/) — **[UNRELEASED]** means not yet tagged/released.
Conventions: `[A]` Added · `[C] `Changed · `[F]` Fixed · `[D]` Deprecated · `[R]` Removed

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
- [ ] Integrate Turnstile verification (TICKET-11, still pending)
- [x] ~~Have I Been Pwned API breach checks~~ — done 2026-07-13 (TICKET-16)
- [ ] Add integration testing via testcontainers-go

# CHANGELOG — controller-api

Format: [Semantic Versioning](https://semver.org/) — **[UNRELEASED]** means not yet tagged/released.
Conventions: `[A]` Added · `[C] `Changed · `[F]` Fixed · `[D]` Deprecated · `[R]` Removed

---

## [UNRELEASED] — 2026-07-13

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

- [ ] Complete GORM models & Postgres repositories for the remaining domains (TestResult, Answer, DeletionRequest, Referral, etc.)
- [x] ~~Complete remaining Auth use cases (Forgot Password, Verify Reset OTP, Reset Password, Logout, Logout-all)~~ — done 2026-07-13, see UNRELEASED entry above (plus unplanned `change-password`)
- [ ] Complete worker implementation (Asynq background workers for PDF, anonymization — email OTP sending done 2026-07-13)
- [ ] Implement Redis-backed Distributed Lock & Idempotency Service
- [ ] Integrate Turnstile verification and actual Have I Been Pwned API checks
- [ ] Add integration testing via testcontainers-go

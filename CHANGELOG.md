# CHANGELOG — controller-api

Format: [Semantic Versioning](https://semver.org/) — **[UNRELEASED]** berarti belum tagged/released.
Konvensi: `[A]` Added · `[C] `Changed · `[F]` Fixed · `[D]` Deprecated · `[R]` Removed

---

## [UNRELEASED] — 2026-07-07

### Scaffolding & Project Foundation

#### [A] Project Structure & Configuration
- Inisialisasi Go module `github.com/aprxty3/your_persona_controller.git` (Go 1.25)
- `go.mod` — dependensi utama: Echo v4, GORM, google/wire, Asynq, go-redis/v9, google/genai (Gemini), google/uuid
- `.air.toml` & `.air.worker.toml` — hot-reload untuk `cmd/api` dan `cmd/worker`
- `.golangci.yml` — linting rules
- `Makefile` — target: `wire`, `migrate`, `run-api`, `run-worker`, `test`, `lint`, `tidy`, `swag`
- `.env.example` — semua env var terdokumentasi per kategori (Server, DB, Redis, S3, SMTP, Gemini, Auth, Turnstile, i18n)

#### [A] cmd/api/main.go
- Graceful shutdown pattern: `SIGTERM`/`SIGINT` → `http.Server.Shutdown(ctx)` dengan timeout dari `SHUTDOWN_TIMEOUT` env (default 30s), sesuai **PRD Section 9.5**
- Placeholder `InitializeAPI` (dikomentari) siap di-uncomment setelah `make wire` berhasil

#### [A] cmd/api/wire.go
- Skeleton Wire injector `InitializeAPI(geminiAPIKey, geminiModel, maxConcurrent)` dengan TODO markers untuk DB, Redis, Asynq providers

---

### Domain Layer — Entities & Repository Interfaces

> Semua domain entity mengikuti aturan **Clean Architecture + DDD**: tidak ada import GORM, HTTP SDK, atau dependency eksternal apa pun. Murni business rules.

#### [A] internal/domain/user/entity.go
- Struct `User` — lengkap sesuai ERD: `ID`, `Email`, `PasswordHash`, `DisplayName`, `Age`, `Status`, `ReferredByCode`, `PreferredLocale`, `EmailVerifiedAt`, `CreatedAt`, `DeletedAt`, `AnonymizedAt`, `TokenVersion`
- Field tambahan untuk auth: `FailedLoginCount`, `LockedUntil` (account-level lockout **FR-H3**)
- Helper method `IsEmailVerified()` dan `IsLocked()`
- Repository interface: `Create`, `FindByID`, `FindByEmail`, `Update`, `IncrementTokenVersion`, `UpdateLoginAttempt`

#### [A] internal/domain/guestsession/entity.go
- Struct `GuestSession` — `SessionID`, `IPHash`, `DisplayName`, `Age`, `Status`, `Locale` (**FR-I2**), `ClaimedByUserID`, `CreatedAt`, `ExpiresAt`
- Helper method `IsClaimed()` dan `IsExpired()`
- Repository interface: `Create`, `FindBySessionID`, `Update`, `FindExpiredUnclaimed`, `DeleteBySessionID`

#### [A] internal/domain/testresult/entity.go
- Typed constants: `ResultStatus` (`processing`, `completed`, `fallback_static`) dan `PDFStatus` (`pending`, `processing`, `completed`, `failed`)
- Struct `TestResult` — lengkap sesuai ERD termasuk semua token usage fields (**FR-C4**) dan `WellbeingFlag` (**FR-B11**)
- Documented XOR ownership invariant (`user_id` XOR `guest_session_id`) sesuai ERD
- Helper method `IsExpired()` — endpoints wajib perlakukan result expired sebagai 404 (**PRD 9.6**)
- Repository interface: `Create`, `FindByID`, `FindByShareToken`, `Update`, `CountMonthlyUsage` (Asia/Jakarta TZ — **PRD 9.1**), `FindExpiredGuestResults`, `UpdatePDFStatus`

#### [A] internal/domain/answer/entity.go
- Struct `Answer` — `ID`, `TestResultID`, `QuestionID`, `Value`, `CreatedAt`, `UpdatedAt`
- Dokumentasi composite UNIQUE constraint `(test_result_id, question_id)` sesuai ERD
- Repository interface: `UpsertAnswers` (ON CONFLICT DO UPDATE — **FR-B10**), `FindByTestResultID`

#### [A] internal/domain/verificationtoken/entity.go
- Typed constant `TokenType` (`email_verification`, `password_reset`)
- Struct `VerificationToken` — `ID`, `UserID`, `Token`, `Type`, `AttemptCount`, `ExpiresAt`, `UsedAt`, `CreatedAt`
- Catatan keamanan kritis: lookup WAJIB scope ke `(user_id, type)` — tidak boleh by token saja (ruang 1 juta kombinasi OTP 6-digit tidak globally unique)
- Repository interface: `Create`, `FindActiveByUserAndType`, `IncrementAttemptCount`, `MarkUsed`, `ExpireAllActiveForUser` (single-token invariant — **AGENTS.md Security Rules**)

#### [A] internal/domain/referral/entity.go
- Typed constant `ReferralEventType` (`signup`, `test_completed`)
- Struct `ReferralCode` — `ID`, `UserID`, `Code`, `CreatedAt`
- Struct `ReferralEvent` — `ID`, `ReferralCodeID`, `ReferredUserID`, `EventType`, `CreatedAt`
- Catatan privasi UU PDP: data yang dikembalikan ke UI WAJIB agregat/masked (**TECHNICAL_DOCUMENTATION Section 5.5**)
- Repository interface: `CreateCode`, `FindCodeByUserID`, `FindCodeByCode`, `CreateEvent`, `CountEventsByCodeID`

#### [A] internal/domain/deletionrequest/entity.go
- Typed constant `DeletionStatus` (`pending_grace`, `processing`, `completed`, `cancelled`)
- Struct `DataDeletionRequest` — `ID`, `UserID`, `NotificationEmail` (snapshot EMAIL sebelum dianonimkan), `Status`, `RequestedAt`, `CompletedAt`
- Repository interface: `Create`, `FindActiveByUserID`, `UpdateStatus`, `FindExpiredGracePeriod`

#### [A] internal/domain/question/entity.go
- Typed constants: `QuestionSection` (`A`, `B`, `C`) dan `QuestionType` (`mc`, `likert`, `essay_prompt`)
- Struct `Question` — locale-agnostic metadata: `ID`, `Section`, `Type`, `IsReverseScored`, `IsAttentionCheck`, `DisplayOrder`
- Struct `QuestionTranslation` — locale-specific: `ID`, `QuestionID`, `Locale`, `QuestionText`, `Options`
- Repository interface: `FindAllWithTranslation` (dengan fallback ke `en` — **FR-I9**), `FindByID`

#### [A] internal/domain/questiontranslation/entity.go
- Struct `QuestionTranslation` — standalone untuk direct lookup per `(question_id, locale)`
- Repository interface: `FindByQuestionAndLocale`, `UpsertTranslation`

#### [A] internal/domain/promptauditlog/entity.go
- Struct `PromptAuditLog` — `ID`, `TestResultID`, `RawPrompt`, `RawResponse`, `FlaggedAnomaly`, `CreatedAt`, `ExpiresAt` (30 hari)
- Catatan keamanan: tabel ini tidak boleh terekspos via API publik
- Repository interface: `Create`, `DeleteByTestResultID` (anonymization worker), `DeleteExpired` (purge cron)

#### [A] internal/domain/insighttemplate/entity.go *(folder baru)*
- Typed constant `ConditionType` (`increase`, `decrease`, `threshold`)
- Struct `InsightTemplate` — `ID`, `InsightKey`, `Locale`, `Trait`, `ConditionType`, `MinDelta`, `ThresholdValue`, `TemplateText`, `IsActive`
- Repository interface: `FindMatchingTemplates` (dengan fallback ke `en` — **FR-I9**)

---

### Infrastructure Layer — Shared Packages

#### [A] pkg/repository/base.go
- Generic `BaseRepository[T any]` dengan operasi CRUD: `Create`, `FindByID`, `Update`, `Delete`
- Semua 10 repository Postgres compose dari sini (DRY — **AGENTS.md DRY Rules**)

#### [A] pkg/httpresponse/response.go
- Envelope response standar: `{ success, data, meta }` untuk sukses; `{ success, error: { code, message } }` untuk gagal
- Fungsi `Success()` dan `Error()` — dipanggil semua handler, tidak ada formatting inline (**TECHNICAL_DOCUMENTATION Section 4.3**)

#### [A] pkg/taskqueue/dispatcher.go *(skeleton)*
- Wrapper Asynq enqueue — dipakai semua job type (PDF, email, anonymize)

#### [A] pkg/locale/ *(skeleton)*
- Locale-fallback resolver — satu fungsi untuk `QUESTION_TRANSLATION` dan `INSIGHT_TEMPLATE` (**FR-I9**)

#### [A] pkg/aivalidator/ *(skeleton)*
- Validasi output Gemini — composable & unit-testable (**FR-C5**)

---

### Infrastructure Layer — Gemini Client

#### [A] internal/infrastructure/gemini/client.go
- `Client` struct dengan semaphore concurrency control (`GEMINI_MAX_CONCURRENT`)
- Implements `assessment.AIGeneratorService` interface
- Context-aware semaphore acquisition (abort jika user disconnect saat waiting — **AGENTS.md Security Rules**)

---

### Interfaces Layer — HTTP

#### [A] internal/interfaces/http/router.go
- Echo router setup dengan global middleware: Recover, Logger, BodyLimit 32KB (**Denial of Wallet protection**)
- Route terdaftar: `POST /v1/assessment/submit`

#### [A] internal/interfaces/http/handler/assessment_handler.go
- Handler `Submit` — extract Idempotency-Key, parse body, extract session/token, delegate ke use case
- Menggunakan `httpresponse.Error` dan `httpresponse.Success` secara konsisten

---

### Application Layer — Use Cases

#### [A] internal/application/assessment/submit_assessment.go
- Struct `SubmitAssessmentUseCase` dengan dependency interfaces: `TestResultRepository`, `AnswerRepository`, `AIGeneratorService`, `DistributedLockService`, `IdempotencyService`, `PDFQueueService`
- Skeleton `Execute()` — outline 9 langkah sesuai **TECHNICAL_DOCUMENTATION Section 5.2**
- 2-phase context cancellation: abort jika user disconnect saat waiting semaphore; `context.WithoutCancel` setelah in-flight (**AGENTS.md**)
- Graceful degradation: fallback ke `fallback_static` jika Gemini gagal (**FR-C2**)

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
- [ ] Complete remaining Auth use cases (Forgot Password, Verify Reset OTP, Reset Password, Logout, Logout-all)
- [ ] Complete worker implementation (Asynq background workers for email, PDF, anonymization)
- [ ] Implement Redis-backed Distributed Lock & Idempotency Service
- [ ] Integrate Turnstile verification and actual Have I Been Pwned API checks
- [ ] Add integration testing via testcontainers-go


package application

import "time"

// MemberMonthlyQuota is the number of assessments a Member may complete per
// calendar month (Asia/Jakarta). Shared between the submit usecase (which
// enforces it) and the dashboard usecase (which derives remaining quota from
// it) so the two never drift out of sync — see PRD Section 5.1's quota table.
const MemberMonthlyQuota = 3

// OTPLength is the digit count of every generated one-time-password —
// email verification and password reset both mint codes via pkg/otp.GenerateOTP(OTPLength).
const OTPLength = 6

// OTPExpiry is how long a generated OTP (email verification or password
// reset) remains valid before ErrOTPExpired applies.
const OTPExpiry = 15 * time.Minute

// PasswordMinLength is the NIST-aligned minimum password length, shared by
// every flow that accepts a new password (register, reset, change).
const PasswordMinLength = 10

// MaxWrongOTPAttempts is the maximum number of invalid OTP entries allowed
// before a token is rejected outright (ErrOTPMaxAttempts), regardless of
// remaining time-to-expiry.
const MaxWrongOTPAttempts = 5

// MinimumAge is the minimum age accepted wherever age is collected — Guest
// onboarding and Member profile completion both enforce this same floor.
const MinimumAge = 13

// GuestDataRetention is how long Guest-owned data survives before being
// purged — GuestSession.ExpiresAt and a Guest-owned TestResult.ExpiresAt are
// both set from this single constant.
// Coincidentally equal to auth.RefreshTokenTTL, but that's an unrelated
// concern (session lifetime, not data retention) — deliberately NOT unified
// with it, since a future change to one must not silently change the other.
const GuestDataRetention = 14 * 24 * time.Hour

// DeletionGracePeriod is how long an account deletion request waits before
// the anonymization worker processes it. Coincidentally also 14
// days like GuestDataRetention, but a distinct business rule (account
// deletion grace period vs. guest data auto-purge) — kept as its own
// constant so the two can diverge independently if either policy changes.
const DeletionGracePeriod = 14 * 24 * time.Hour

// GritTrendPoints caps how many recent results feed the GRIT trend  —
// a handful of points is enough for a sparkline, and keeps the query cheap.
const GritTrendPoints = 5

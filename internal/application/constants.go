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

// OTPExpiryMinutes is how long a generated OTP (email verification or
// password reset) remains valid before ErrOTPExpired applies.
const OTPExpiryMinutes = 15

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

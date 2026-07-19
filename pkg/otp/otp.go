// Package otp generates the numeric one-time-passwords used for email verification and password reset.
package otp

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// GenerateOTP produces a zero-padded, cryptographically secure numeric OTP of the given length.
func GenerateOTP(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("otp: length must be greater than zero")
	}

	// Create secure random number up to 10^length
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(length)), nil)
	num, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", fmt.Errorf("otp: generate secure random: %w", err)
	}

	return fmt.Sprintf("%0*d", length, num), nil
}

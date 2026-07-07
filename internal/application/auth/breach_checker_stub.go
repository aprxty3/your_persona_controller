package auth

import "context"

// NoopBreachChecker always reports passwords as NOT breached.
// Suitable for development and test environments.
type NoopBreachChecker struct{}

// NewNoopBreachChecker creates a new NoopBreachChecker.
func NewNoopBreachChecker() PasswordBreachChecker {
	return &NoopBreachChecker{}
}

// IsBreached mocks the HIBP check by always returning false.
func (c *NoopBreachChecker) IsBreached(_ context.Context, _ string) (bool, error) {
	return false, nil
}

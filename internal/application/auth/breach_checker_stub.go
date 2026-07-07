package auth

import "context"

// It always reports passwords as NOT breached — suitable for dev/test environments.
type NoopBreachChecker struct{}

func NewNoopBreachChecker() PasswordBreachChecker {
	return &NoopBreachChecker{}
}

func (c *NoopBreachChecker) IsBreached(_ context.Context, _ string) (bool, error) {
	return false, nil
}

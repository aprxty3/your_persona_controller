package middleware

import "github.com/labstack/echo/v4"

// NoIndex sets X-Robots-Tag: noindex, nofollow — required on any route serving
// personal data that must never be crawled/indexed.
// Apply at the route group level so every current and future sub-route inherits it automatically.
func NoIndex(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set("X-Robots-Tag", "noindex, nofollow")
		return next(c)
	}
}

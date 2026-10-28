// Package config supplies only the options needed by historical API fixtures.
// The active application's environment loader is yember-core/config.
package config

type Config struct {
	JWTSecret          string
	CORSAllowedOrigins string
	AdminCookieSecure  bool
	PasswordResetURL   string
	ResendAPIKey       string
	EmailFrom          string
}

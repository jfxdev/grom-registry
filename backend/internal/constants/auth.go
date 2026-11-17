package constants

import "time"

const (
	SessionCookieName           = "grom_session"
	DefaultSessionHours         = 24
	DefaultRegistryTokenTTL     = 5
	DefaultPasswordResetMinutes = 30
	MinimumPasswordLength       = 12
	PrincipalUser               = "user"
	PrincipalServiceAccount     = "service_account"
)

const (
	// APIKeyVerificationCacheTTL bounds how long a successful Argon2id access-key
	// verification may be reused. Revocation, expiry and owner state are still
	// read on every request; only the hash computation is skipped.
	APIKeyVerificationCacheTTL     = 60 * time.Second
	APIKeyVerificationCacheEntries = 1024
)

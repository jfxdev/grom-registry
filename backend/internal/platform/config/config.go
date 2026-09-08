package config

import (
	"fmt"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
)

type DeploymentProfile string

const (
	DeploymentProfileDevelopment DeploymentProfile = "development"
	DeploymentProfilePermissive  DeploymentProfile = "permissive"
	DeploymentProfileStrict      DeploymentProfile = "strict"
)

type Config struct {
	HTTPAddress               string
	DatabaseURL               string
	RegistryURL               string
	PublicURL                 string
	DeploymentProfile         DeploymentProfile
	InsecureHTTP              bool
	DataDir                   string
	SigningKeyPath            string
	SigningCertPath           string
	SigningJWKSPath           string
	BootstrapEmail            string
	BootstrapUsername         string
	BootstrapPassword         string
	SessionTTL                time.Duration
	RegistryTokenTTL          time.Duration
	SecureCookies             bool
	EnableAPIDocs             bool
	MigrationLockWait         time.Duration
	TrustedProxies            []netip.Prefix
	AuthFailureLimit          int
	AuthFailureWindow         time.Duration
	AuthBlockDuration         time.Duration
	BackupAgentSocket         string
	RegistryMaintenanceSocket string
	SMTP                      SMTPConfig
}

type SMTPTLSMode string

const (
	SMTPTLSModeStartTLS SMTPTLSMode = "starttls"
	SMTPTLSModeTLS      SMTPTLSMode = "tls"
)

// SMTPConfig is empty when outbound email delivery is disabled. SMTP is kept
// separate from Identity state: credentials remain deployment configuration and
// are never persisted or exposed through the management API.
type SMTPConfig struct {
	Enabled     bool
	Host        string
	Port        int
	FromAddress string
	TLSMode     SMTPTLSMode
	Username    string
	Password    string
}

func Load() (Config, error) {
	dataDir := env("GROM_DATA_DIR", "./data")
	deploymentProfile, err := deploymentProfileEnv("GROM_DEPLOYMENT_PROFILE")
	if err != nil {
		return Config{}, err
	}
	secureCookies, err := strictBoolEnv("GROM_SECURE_COOKIES", false)
	if err != nil {
		return Config{}, err
	}
	allowInsecurePrivateHTTP, err := strictBoolEnv("GROM_ALLOW_INSECURE_PRIVATE_HTTP", false)
	if err != nil {
		return Config{}, err
	}
	enableAPIDocs, err := strictBoolEnv("GROM_ENABLE_API_DOCS", true)
	if err != nil {
		return Config{}, err
	}
	trustedProxies, err := trustedProxiesEnv("GROM_TRUSTED_PROXIES")
	if err != nil {
		return Config{}, err
	}
	if legacyTrust, legacyErr := strictBoolEnv("GROM_TRUST_PROXY_HEADERS", false); legacyErr != nil {
		return Config{}, legacyErr
	} else if legacyTrust {
		return Config{}, fmt.Errorf("GROM_TRUST_PROXY_HEADERS is unsafe and no longer supported; configure GROM_TRUSTED_PROXIES with explicit IP addresses or CIDR ranges")
	}
	authFailureLimit, err := positiveIntEnv("GROM_AUTH_FAILURE_LIMIT", 5)
	if err != nil {
		return Config{}, err
	}
	authFailureWindow, err := strictDurationEnv("GROM_AUTH_FAILURE_WINDOW", 5*time.Minute)
	if err != nil {
		return Config{}, err
	}
	authBlockDuration, err := strictDurationEnv("GROM_AUTH_BLOCK_DURATION", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	smtp, err := smtpConfigFromEnv()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		HTTPAddress:               env("GROM_HTTP_ADDRESS", ":8080"),
		DatabaseURL:               env("GROM_DATABASE_URL", "sqlite://"+dataDir+"/grom.db"),
		RegistryURL:               env("GROM_REGISTRY_URL", "http://distribution:5000"),
		PublicURL:                 strings.TrimRight(env("GROM_PUBLIC_URL", "http://localhost:8080"), "/"),
		DeploymentProfile:         deploymentProfile,
		DataDir:                   dataDir,
		SigningKeyPath:            env("GROM_SIGNING_KEY_PATH", dataDir+"/signing-key.pem"),
		SigningCertPath:           env("GROM_SIGNING_CERT_PATH", dataDir+"/signing-cert.pem"),
		SigningJWKSPath:           env("GROM_SIGNING_JWKS_PATH", dataDir+"/jwks.json"),
		BootstrapEmail:            strings.ToLower(strings.TrimSpace(env("GROM_BOOTSTRAP_ADMIN_EMAIL", "admin@grom.local"))),
		BootstrapUsername:         strings.TrimSpace(env("GROM_BOOTSTRAP_ADMIN_USERNAME", "admin")),
		BootstrapPassword:         env("GROM_BOOTSTRAP_ADMIN_PASSWORD", ""),
		SessionTTL:                durationEnv("GROM_SESSION_TTL", constants.DefaultSessionHours*time.Hour),
		RegistryTokenTTL:          durationEnv("GROM_REGISTRY_TOKEN_TTL", constants.DefaultRegistryTokenTTL*time.Minute),
		SecureCookies:             secureCookies,
		EnableAPIDocs:             enableAPIDocs,
		MigrationLockWait:         durationEnv("GROM_MIGRATION_LOCK_TIMEOUT", 30*time.Second),
		TrustedProxies:            trustedProxies,
		AuthFailureLimit:          authFailureLimit,
		AuthFailureWindow:         authFailureWindow,
		AuthBlockDuration:         authBlockDuration,
		BackupAgentSocket:         env("GROM_BACKUP_AGENT_SOCKET", ""),
		RegistryMaintenanceSocket: env("GROM_REGISTRY_MAINTENANCE_SOCKET", ""),
		SMTP:                      smtp,
	}
	if cfg.BootstrapEmail == "" || cfg.BootstrapUsername == "" || cfg.BootstrapPassword == "" {
		return Config{}, fmt.Errorf("bootstrap administrator credentials cannot be empty")
	}
	insecureHTTP, err := validatePublicSecurity(
		cfg.PublicURL, cfg.SecureCookies, cfg.DeploymentProfile, allowInsecurePrivateHTTP,
	)
	if err != nil {
		return Config{}, err
	}
	cfg.InsecureHTTP = insecureHTTP
	return cfg, nil
}

func smtpConfigFromEnv() (SMTPConfig, error) {
	host := strings.TrimSpace(os.Getenv("GROM_SMTP_HOST"))
	portRaw := strings.TrimSpace(os.Getenv("GROM_SMTP_PORT"))
	fromAddress := strings.TrimSpace(os.Getenv("GROM_SMTP_FROM_ADDRESS"))
	tlsModeRaw := strings.ToLower(strings.TrimSpace(os.Getenv("GROM_SMTP_TLS_MODE")))
	username := os.Getenv("GROM_SMTP_USERNAME")
	password := os.Getenv("GROM_SMTP_PASSWORD")

	if host == "" {
		if portRaw != "" || fromAddress != "" || tlsModeRaw != "" || username != "" || password != "" {
			return SMTPConfig{}, fmt.Errorf("GROM_SMTP_HOST is required when SMTP settings are configured")
		}
		return SMTPConfig{}, nil
	}
	if !validSMTPHost(host) {
		return SMTPConfig{}, fmt.Errorf("GROM_SMTP_HOST must be a hostname or IP address without a scheme or path")
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil || port < 1 || port > 65535 {
		return SMTPConfig{}, fmt.Errorf("GROM_SMTP_PORT must be an integer between 1 and 65535")
	}
	parsedFrom, err := mail.ParseAddress(fromAddress)
	if err != nil || parsedFrom.Address != fromAddress {
		return SMTPConfig{}, fmt.Errorf("GROM_SMTP_FROM_ADDRESS must be a mailbox address without a display name")
	}
	tlsMode := SMTPTLSMode(tlsModeRaw)
	if tlsMode != SMTPTLSModeStartTLS && tlsMode != SMTPTLSModeTLS {
		return SMTPConfig{}, fmt.Errorf("GROM_SMTP_TLS_MODE must be starttls or tls")
	}
	if (username == "") != (password == "") {
		return SMTPConfig{}, fmt.Errorf("GROM_SMTP_USERNAME and GROM_SMTP_PASSWORD must be configured together")
	}
	return SMTPConfig{
		Enabled: true, Host: host, Port: port, FromAddress: parsedFrom.Address,
		TLSMode: tlsMode, Username: username, Password: password,
	}, nil
}

func validSMTPHost(host string) bool {
	if strings.Contains(host, "://") || strings.ContainsAny(host, "/@ \t\r\n") {
		return false
	}
	address := host
	if strings.HasPrefix(host, "[") || strings.HasSuffix(host, "]") {
		if !strings.HasPrefix(host, "[") || !strings.HasSuffix(host, "]") {
			return false
		}
		address = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if net.ParseIP(address) != nil {
		return true
	}
	if address == "" || len(address) > 253 || strings.Contains(address, ":") {
		return false
	}
	for _, label := range strings.Split(address, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
				(character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func strictBoolEnv(key string, fallback bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}

func strictDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return value, nil
}

func positiveIntEnv(key string, fallback int) (int, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func deploymentProfileEnv(key string) (DeploymentProfile, error) {
	value := DeploymentProfile(strings.ToLower(strings.TrimSpace(env(key, string(DeploymentProfileStrict)))))
	switch value {
	case DeploymentProfileDevelopment, DeploymentProfilePermissive, DeploymentProfileStrict:
		return value, nil
	default:
		return "", fmt.Errorf("%s must be development, permissive, or strict", key)
	}
}

func trustedProxiesEnv(key string) ([]netip.Prefix, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, nil
	}
	values := strings.Split(raw, ",")
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		candidate := strings.TrimSpace(value)
		prefix, err := netip.ParsePrefix(candidate)
		if err != nil {
			address, addressErr := netip.ParseAddr(candidate)
			if addressErr != nil {
				return nil, fmt.Errorf("%s contains invalid address or CIDR %q", key, candidate)
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func validatePublicSecurity(
	rawPublicURL string,
	secureCookies bool,
	profile DeploymentProfile,
	allowInsecurePrivateHTTP bool,
) (bool, error) {
	publicURL, err := url.Parse(rawPublicURL)
	if err != nil || publicURL.Scheme == "" || publicURL.Host == "" {
		return false, fmt.Errorf("GROM_PUBLIC_URL must be an absolute HTTP or HTTPS URL")
	}
	if publicURL.Scheme != "http" && publicURL.Scheme != "https" {
		return false, fmt.Errorf("GROM_PUBLIC_URL must use http or https")
	}
	if publicURL.User != nil || (publicURL.Path != "" && publicURL.Path != "/") ||
		publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return false, fmt.Errorf("GROM_PUBLIC_URL must not contain credentials, a path, a query, or a fragment")
	}
	if allowInsecurePrivateHTTP && profile != DeploymentProfilePermissive {
		return false, fmt.Errorf("GROM_ALLOW_INSECURE_PRIVATE_HTTP is valid only when GROM_DEPLOYMENT_PROFILE=permissive")
	}

	if publicURL.Scheme == "https" {
		if !secureCookies {
			return false, fmt.Errorf("GROM_SECURE_COOKIES must be true when GROM_PUBLIC_URL uses https")
		}
		return false, nil
	}

	host := publicURL.Hostname()
	switch profile {
	case DeploymentProfileDevelopment:
		if !isLoopbackHost(host) {
			return false, fmt.Errorf("development GROM_PUBLIC_URL may use http only on a loopback address")
		}
		return false, nil
	case DeploymentProfilePermissive:
		if !allowInsecurePrivateHTTP {
			return false, fmt.Errorf("permissive HTTP requires GROM_ALLOW_INSECURE_PRIVATE_HTTP=true")
		}
		if !isPrivateHTTPHost(host) {
			return false, fmt.Errorf("permissive HTTP is allowed only for loopback, private, link-local, .home.arpa, or .local addresses")
		}
		return true, nil
	case DeploymentProfileStrict:
		return false, fmt.Errorf("strict GROM_PUBLIC_URL must use https")
	default:
		return false, fmt.Errorf("unsupported deployment profile %q", profile)
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
}

func isPrivateHTTPHost(host string) bool {
	normalized := strings.ToLower(strings.TrimSuffix(host, "."))
	if isLoopbackHost(normalized) ||
		strings.HasSuffix(normalized, ".home.arpa") ||
		strings.HasSuffix(normalized, ".local") {
		return true
	}
	address, err := netip.ParseAddr(normalized)
	if err != nil {
		return false
	}
	return address.IsPrivate() || address.IsLinkLocalUnicast()
}

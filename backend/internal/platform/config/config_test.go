package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadAcceptsLocalDevelopmentSecurityDefaults(t *testing.T) {
	setRequiredEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthFailureLimit != 5 {
		t.Fatalf("expected default failure limit 5, got %d", cfg.AuthFailureLimit)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Fatalf("expected no trusted proxies, got %v", cfg.TrustedProxies)
	}
	if cfg.DeploymentProfile != DeploymentProfileDevelopment || cfg.InsecureHTTP {
		t.Fatalf("expected safe development profile, got profile=%q insecure=%t", cfg.DeploymentProfile, cfg.InsecureHTTP)
	}
}

func TestLoadDefaultsToStrictProfile(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("GROM_DEPLOYMENT_PROFILE", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "must be development") {
		t.Fatalf("expected empty explicit profile to be rejected, got %v", err)
	}

	t.Setenv("GROM_DEPLOYMENT_PROFILE", "strict")
	t.Setenv("GROM_PUBLIC_URL", "https://registry.example.com")
	t.Setenv("GROM_SECURE_COOKIES", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DeploymentProfile != DeploymentProfileStrict {
		t.Fatalf("expected strict profile, got %q", cfg.DeploymentProfile)
	}
}

func TestDeploymentProfileEnvDefaultsToStrictWhenAbsent(t *testing.T) {
	t.Setenv("GROM_DEPLOYMENT_PROFILE", "")
	if err := os.Unsetenv("GROM_DEPLOYMENT_PROFILE"); err != nil {
		t.Fatal(err)
	}

	profile, err := deploymentProfileEnv("GROM_DEPLOYMENT_PROFILE")
	if err != nil {
		t.Fatal(err)
	}
	if profile != DeploymentProfileStrict {
		t.Fatalf("expected strict default, got %q", profile)
	}
}

func TestLoadEnforcesDeploymentProfileSecurity(t *testing.T) {
	tests := []struct {
		name          string
		profile       string
		publicURL     string
		secureCookies string
		allowHTTP     string
		wantInsecure  bool
		wantError     string
	}{
		{
			name:          "strict rejects localhost HTTP",
			profile:       "strict",
			publicURL:     "http://localhost:8080",
			secureCookies: "false",
			wantError:     "must use https",
		},
		{
			name:          "strict accepts proxy-fronted HTTPS",
			profile:       "strict",
			publicURL:     "https://registry.example.com",
			secureCookies: "true",
		},
		{
			name:          "development accepts loopback IPv4 HTTP",
			profile:       "development",
			publicURL:     "http://127.0.0.1:8080",
			secureCookies: "false",
		},
		{
			name:          "development accepts loopback IPv6 HTTP",
			profile:       "development",
			publicURL:     "http://[::1]:8080",
			secureCookies: "false",
		},
		{
			name:          "development rejects private HTTP",
			profile:       "development",
			publicURL:     "http://192.168.1.20:8080",
			secureCookies: "false",
			wantError:     "loopback",
		},
		{
			name:          "permissive private HTTP requires opt-in",
			profile:       "permissive",
			publicURL:     "http://192.168.1.20:8080",
			secureCookies: "false",
			wantError:     "requires GROM_ALLOW_INSECURE_PRIVATE_HTTP=true",
		},
		{
			name:          "permissive accepts RFC1918 HTTP with opt-in",
			profile:       "permissive",
			publicURL:     "http://172.16.20.30:8080",
			secureCookies: "false",
			allowHTTP:     "true",
			wantInsecure:  true,
		},
		{
			name:          "permissive accepts local namespace HTTP with opt-in",
			profile:       "permissive",
			publicURL:     "http://registry.home.arpa:8080",
			secureCookies: "false",
			allowHTTP:     "true",
			wantInsecure:  true,
		},
		{
			name:          "permissive accepts IPv6 unique local HTTP with opt-in",
			profile:       "permissive",
			publicURL:     "http://[fd00::20]:8080",
			secureCookies: "false",
			allowHTTP:     "true",
			wantInsecure:  true,
		},
		{
			name:          "permissive rejects public HTTP despite opt-in",
			profile:       "permissive",
			publicURL:     "http://registry.example.com",
			secureCookies: "false",
			allowHTTP:     "true",
			wantError:     "allowed only",
		},
		{
			name:          "HTTPS without secure cookies",
			profile:       "permissive",
			publicURL:     "https://registry.example.com",
			secureCookies: "false",
			wantError:     "must be true",
		},
		{
			name:          "permissive accepts secure public URL",
			profile:       "permissive",
			publicURL:     "https://registry.example.com",
			secureCookies: "true",
		},
		{
			name:          "insecure opt-in is rejected outside permissive",
			profile:       "development",
			publicURL:     "http://localhost:8080",
			secureCookies: "false",
			allowHTTP:     "true",
			wantError:     "valid only",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv("GROM_DEPLOYMENT_PROFILE", test.profile)
			t.Setenv("GROM_PUBLIC_URL", test.publicURL)
			t.Setenv("GROM_SECURE_COOKIES", test.secureCookies)
			allowHTTP := test.allowHTTP
			if allowHTTP == "" {
				allowHTTP = "false"
			}
			t.Setenv("GROM_ALLOW_INSECURE_PRIVATE_HTTP", allowHTTP)

			cfg, err := Load()
			if test.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				if cfg.InsecureHTTP != test.wantInsecure {
					t.Fatalf("expected insecure=%t, got %t", test.wantInsecure, cfg.InsecureHTTP)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("expected error containing %q, got %v", test.wantError, err)
			}
		})
	}
}

func TestLoadParsesExplicitTrustedProxies(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("GROM_TRUSTED_PROXIES", "10.0.0.0/8, 192.0.2.10, 2001:db8::/32")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TrustedProxies) != 3 {
		t.Fatalf("expected three trusted proxy ranges, got %v", cfg.TrustedProxies)
	}
}

func TestLoadRejectsMalformedDeploymentConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		wantError string
	}{
		{
			name:      "unknown profile",
			key:       "GROM_DEPLOYMENT_PROFILE",
			value:     "production",
			wantError: "must be development, permissive, or strict",
		},
		{
			name:      "malformed public URL",
			key:       "GROM_PUBLIC_URL",
			value:     "://registry",
			wantError: "absolute HTTP or HTTPS URL",
		},
		{
			name:      "public URL with credentials",
			key:       "GROM_PUBLIC_URL",
			value:     "http://admin:secret@localhost:8080",
			wantError: "must not contain credentials",
		},
		{
			name:      "invalid insecure opt-in",
			key:       "GROM_ALLOW_INSECURE_PRIVATE_HTTP",
			value:     "sometimes",
			wantError: "must be true or false",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv(test.key, test.value)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("expected error containing %q, got %v", test.wantError, err)
			}
		})
	}
}

func TestLoadRejectsLegacyUnboundedProxyTrust(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("GROM_TRUST_PROXY_HEADERS", "true")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "no longer supported") {
		t.Fatalf("expected legacy proxy trust error, got %v", err)
	}
}

func TestLoadSMTPConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     string
		from     string
		tlsMode  string
		username string
		password string
		wantErr  string
		wantMode SMTPTLSMode
		wantAuth bool
	}{
		{name: "disabled by default"},
		{name: "mandatory STARTTLS relay", host: "smtp.example.test", port: "587", from: "notifications@example.test", tlsMode: "starttls", wantMode: SMTPTLSModeStartTLS},
		{name: "implicit TLS authenticated relay", host: "smtp.example.test", port: "465", from: "notifications@example.test", tlsMode: "tls", username: "mailer", password: "app-password", wantMode: SMTPTLSModeTLS, wantAuth: true},
		{name: "partial configuration", port: "587", wantErr: "GROM_SMTP_HOST is required"},
		{name: "insecure SMTP mode", host: "smtp.example.test", port: "25", from: "notifications@example.test", tlsMode: "none", wantErr: "must be starttls or tls"},
		{name: "invalid host URL", host: "https://smtp.example.test", port: "465", from: "notifications@example.test", tlsMode: "tls", wantErr: "hostname or IP"},
		{name: "host must not include a port", host: "smtp.example.test:465", port: "465", from: "notifications@example.test", tlsMode: "tls", wantErr: "hostname or IP"},
		{name: "invalid port", host: "smtp.example.test", port: "0", from: "notifications@example.test", tlsMode: "tls", wantErr: "between 1 and 65535"},
		{name: "display name sender is rejected", host: "smtp.example.test", port: "465", from: "Grom <notifications@example.test>", tlsMode: "tls", wantErr: "mailbox address"},
		{name: "incomplete credentials", host: "smtp.example.test", port: "465", from: "notifications@example.test", tlsMode: "tls", username: "mailer", wantErr: "configured together"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv("GROM_SMTP_HOST", test.host)
			t.Setenv("GROM_SMTP_PORT", test.port)
			t.Setenv("GROM_SMTP_FROM_ADDRESS", test.from)
			t.Setenv("GROM_SMTP_TLS_MODE", test.tlsMode)
			t.Setenv("GROM_SMTP_USERNAME", test.username)
			t.Setenv("GROM_SMTP_PASSWORD", test.password)

			cfg, err := Load()
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if test.host == "" {
				if cfg.SMTP.Enabled {
					t.Fatalf("expected SMTP to be disabled, got %#v", cfg.SMTP)
				}
				return
			}
			if !cfg.SMTP.Enabled || cfg.SMTP.TLSMode != test.wantMode || (cfg.SMTP.Username != "") != test.wantAuth {
				t.Fatalf("unexpected SMTP config %#v", cfg.SMTP)
			}
		})
	}
}

func TestLoadStrictProfileRejectsInsecureSMTPTransport(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("GROM_DEPLOYMENT_PROFILE", "strict")
	t.Setenv("GROM_PUBLIC_URL", "https://registry.example.test")
	t.Setenv("GROM_SECURE_COOKIES", "true")
	t.Setenv("GROM_SMTP_HOST", "smtp.example.test")
	t.Setenv("GROM_SMTP_PORT", "25")
	t.Setenv("GROM_SMTP_FROM_ADDRESS", "notifications@example.test")
	t.Setenv("GROM_SMTP_TLS_MODE", "none")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "GROM_SMTP_TLS_MODE must be starttls or tls") {
		t.Fatalf("expected strict profile to reject insecure SMTP, got %v", err)
	}
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("GROM_BOOTSTRAP_ADMIN_PASSWORD", "test-password")
	t.Setenv("GROM_DEPLOYMENT_PROFILE", "development")
	t.Setenv("GROM_PUBLIC_URL", "http://localhost:8080")
	t.Setenv("GROM_SECURE_COOKIES", "false")
	t.Setenv("GROM_ALLOW_INSECURE_PRIVATE_HTTP", "false")
	t.Setenv("GROM_ENABLE_API_DOCS", "true")
	t.Setenv("GROM_TRUST_PROXY_HEADERS", "false")
	t.Setenv("GROM_TRUSTED_PROXIES", "")
	t.Setenv("GROM_AUTH_FAILURE_LIMIT", "5")
	t.Setenv("GROM_AUTH_FAILURE_WINDOW", "5m")
	t.Setenv("GROM_AUTH_BLOCK_DURATION", "15m")
}

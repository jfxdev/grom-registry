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

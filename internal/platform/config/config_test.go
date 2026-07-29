package config

import (
	"strings"
	"testing"
)

func validEnvironment() []string {
	return []string{
		"DIFY_AGENT_ENVIRONMENT=test",
		"DIFY_AGENT_DATABASE_URL=postgres://user:secret@localhost/db?sslmode=require",
		"DIFY_AGENT_BOOTSTRAP_ADMIN_EMAIL=admin@example.test",
		"DIFY_AGENT_BOOTSTRAP_ADMIN_PASSWORD=a-long-development-password",
		"DIFY_AGENT_CORS_ALLOWED_ORIGINS=https://app.example.test",
		"DIFY_AGENT_RELEASE_ID=r1",
		"DIFY_AGENT_BUILD_COMMIT=abc",
	}
}

func replaceEnvironment(environment []string, key, value string) []string {
	result := append([]string(nil), environment...)
	for i, item := range result {
		if strings.HasPrefix(item, key+"=") {
			result[i] = key + "=" + value
			return result
		}
	}
	return append(result, key+"="+value)
}

func TestLoadValidDevelopmentConfig(t *testing.T) {
	cfg, err := load([]string{
		"DIFY_AGENT_ENVIRONMENT=development",
		"DIFY_AGENT_DATABASE_URL=postgres://user:pass@localhost/db",
		"DIFY_AGENT_BOOTSTRAP_ADMIN_EMAIL=admin@example.test",
		"DIFY_AGENT_BOOTSTRAP_ADMIN_PASSWORD=a-long-development-password",
		"DIFY_AGENT_CORS_ALLOWED_ORIGINS=http://localhost:5173,https://app.example.com",
		"DIFY_AGENT_RELEASE_ID=dev",
		"DIFY_AGENT_BUILD_COMMIT=dirty",
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DatabaseMaxConns != 10 || len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadRejectsUnknownNamespacedVariable(t *testing.T) {
	_, err := load([]string{"DIFY_AGENT_TYPO=value"})
	if err == nil {
		t.Fatal("expected unknown variable to fail")
	}
}

func TestLoadRejectsInsecureProductionOrigin(t *testing.T) {
	_, err := load([]string{
		"DIFY_AGENT_ENVIRONMENT=production", "DIFY_AGENT_DATABASE_URL=postgres://user:secret@localhost/db?sslmode=verify-full",
		"DIFY_AGENT_BOOTSTRAP_ADMIN_EMAIL=admin@example.test", "DIFY_AGENT_BOOTSTRAP_ADMIN_PASSWORD=a-long-development-password",
		"DIFY_AGENT_OTEL_EXPORTER_OTLP_ENDPOINT=https://telemetry.example.test",
		"DIFY_AGENT_CORS_ALLOWED_ORIGINS=http://localhost:5173", "DIFY_AGENT_RELEASE_ID=r1", "DIFY_AGENT_BUILD_COMMIT=abc",
	})
	if err == nil {
		t.Fatal("expected insecure production origin to fail")
	}
}

func TestLoadRejectsInvalidBootstrapAdminCredentials(t *testing.T) {
	base := []string{
		"DIFY_AGENT_ENVIRONMENT=test", "DIFY_AGENT_DATABASE_URL=postgres://user:secret@localhost/db?sslmode=require",
		"DIFY_AGENT_CORS_ALLOWED_ORIGINS=https://app.example.test", "DIFY_AGENT_RELEASE_ID=r1", "DIFY_AGENT_BUILD_COMMIT=abc",
	}
	tests := [][]string{
		append(base, "DIFY_AGENT_BOOTSTRAP_ADMIN_EMAIL=invalid", "DIFY_AGENT_BOOTSTRAP_ADMIN_PASSWORD=a-long-test-password"),
		append(base, "DIFY_AGENT_BOOTSTRAP_ADMIN_EMAIL=admin@example.test", "DIFY_AGENT_BOOTSTRAP_ADMIN_PASSWORD=short"),
	}
	for _, environment := range tests {
		if _, err := load(environment); err == nil {
			t.Fatal("expected invalid administrator credentials to fail")
		}
	}
}

func TestLoadRejectsInvalidEnvironment(t *testing.T) {
	if _, err := load(replaceEnvironment(validEnvironment(), "DIFY_AGENT_ENVIRONMENT", "preview")); err == nil {
		t.Fatal("expected unknown environment to fail")
	}
}

func TestLoadRejectsInvalidDatabaseURLWithoutLeakingSecret(t *testing.T) {
	const secretURL = "mysql://admin:highly-sensitive@localhost/db"
	_, err := load(replaceEnvironment(validEnvironment(), "DIFY_AGENT_DATABASE_URL", secretURL))
	if err == nil {
		t.Fatal("expected non-PostgreSQL URL to fail")
	}
	if strings.Contains(err.Error(), secretURL) || strings.Contains(err.Error(), "highly-sensitive") {
		t.Fatalf("configuration error leaked database credentials: %v", err)
	}
}

func TestLoadRejectsDatabaseURLWithoutPassword(t *testing.T) {
	if _, err := load(replaceEnvironment(validEnvironment(), "DIFY_AGENT_DATABASE_URL", "postgres://user@localhost/db?sslmode=require")); err == nil {
		t.Fatal("expected database URL without a password to fail")
	}
}

func TestLoadRequiresVerifiedProductionDatabaseTLS(t *testing.T) {
	for _, sslMode := range []string{"", "disable", "require", "verify-ca"} {
		t.Run("sslmode="+sslMode, func(t *testing.T) {
			environment := replaceEnvironment(validEnvironment(), "DIFY_AGENT_ENVIRONMENT", "production")
			environment = replaceEnvironment(environment, "DIFY_AGENT_DATABASE_URL", "postgres://user:secret@db.example.test/app?sslmode="+sslMode)
			environment = replaceEnvironment(environment, "DIFY_AGENT_OTEL_EXPORTER_OTLP_ENDPOINT", "https://telemetry.example.test")
			if _, err := load(environment); err == nil {
				t.Fatalf("expected production sslmode %q to fail", sslMode)
			}
		})
	}
}

func TestLoadAcceptsVerifiedProductionDatabaseTLS(t *testing.T) {
	environment := replaceEnvironment(validEnvironment(), "DIFY_AGENT_ENVIRONMENT", "production")
	environment = replaceEnvironment(environment, "DIFY_AGENT_DATABASE_URL", "postgres://user:secret@db.example.test/app?sslmode=verify-full")
	environment = replaceEnvironment(environment, "DIFY_AGENT_OTEL_EXPORTER_OTLP_ENDPOINT", "https://telemetry.example.test")
	if _, err := load(environment); err != nil {
		t.Fatalf("expected verified production TLS to pass: %v", err)
	}
}

func TestLoadRejectsInvalidAddressPoolAndDurations(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{key: "DIFY_AGENT_HTTP_ADDRESS", value: "localhost"},
		{key: "DIFY_AGENT_HTTP_ADDRESS", value: "localhost:99999"},
		{key: "DIFY_AGENT_DATABASE_MAX_CONNS", value: "1001"},
		{key: "DIFY_AGENT_DATABASE_MIN_CONNS", value: "11"},
		{key: "DIFY_AGENT_HTTP_READ_TIMEOUT", value: "0s"},
		{key: "DIFY_AGENT_REQUEST_TIMEOUT", value: "invalid"},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			if _, err := load(replaceEnvironment(validEnvironment(), test.key, test.value)); err == nil {
				t.Fatalf("expected %s=%q to fail", test.key, test.value)
			}
		})
	}
}

func TestLoadRejectsCORSURLWithPath(t *testing.T) {
	if _, err := load(replaceEnvironment(validEnvironment(), "DIFY_AGENT_CORS_ALLOWED_ORIGINS", "https://app.example.test/api")); err == nil {
		t.Fatal("expected CORS URL with path to fail")
	}
}

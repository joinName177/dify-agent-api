package config

import "testing"

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
		"DIFY_AGENT_ENVIRONMENT=production", "DIFY_AGENT_DATABASE_URL=postgres://localhost/db",
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
		"DIFY_AGENT_ENVIRONMENT=test", "DIFY_AGENT_DATABASE_URL=postgres://localhost/db",
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

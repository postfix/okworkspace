package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return p
}

func TestLoadValidConfig(t *testing.T) {
	p := writeTempConfig(t, `
server:
  listen: "127.0.0.1:9090"
  public_url: "https://wiki.example.local"
storage:
  data_dir: "/tmp/okf-data"
  repo_dir: "/tmp/okf-data/repo"
auth:
  session_cookie_name: "custom_cookie"
  session_ttl_hours: 72
admin:
  username: "root"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Listen != "127.0.0.1:9090" {
		t.Errorf("server.listen = %q, want 127.0.0.1:9090", cfg.Server.Listen)
	}
	if cfg.Storage.DataDir != "/tmp/okf-data" {
		t.Errorf("storage.data_dir = %q", cfg.Storage.DataDir)
	}
	if cfg.Auth.SessionCookieName != "custom_cookie" {
		t.Errorf("session_cookie_name = %q", cfg.Auth.SessionCookieName)
	}
	if cfg.Auth.SessionTTLHours != 72 {
		t.Errorf("session_ttl_hours = %d, want 72", cfg.Auth.SessionTTLHours)
	}
	if cfg.Admin.Username != "root" {
		t.Errorf("admin.username = %q, want root", cfg.Admin.Username)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	p := writeTempConfig(t, `
server:
  listen: "0.0.0.0:8080"
storage:
  data_dir: "/tmp/okf-data"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.SessionCookieName != DefaultSessionCookieName {
		t.Errorf("default cookie name = %q, want %q", cfg.Auth.SessionCookieName, DefaultSessionCookieName)
	}
	if cfg.Auth.SessionTTLHours != DefaultSessionTTLHours {
		t.Errorf("default ttl = %d, want %d", cfg.Auth.SessionTTLHours, DefaultSessionTTLHours)
	}
	if cfg.Admin.Username != DefaultAdminUsername {
		t.Errorf("default admin username = %q, want %q", cfg.Admin.Username, DefaultAdminUsername)
	}
}

func TestLoadEnvOverridesAdminUsername(t *testing.T) {
	p := writeTempConfig(t, `
server:
  listen: "0.0.0.0:8080"
storage:
  data_dir: "/tmp/okf-data"
admin:
  username: "fileadmin"
`)
	t.Setenv(EnvAdminUsername, "envadmin")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Admin.Username != "envadmin" {
		t.Errorf("admin username = %q, want envadmin (env override)", cfg.Admin.Username)
	}
}

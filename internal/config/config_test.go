package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestLoadFullSchema(t *testing.T) {
	p := writeTempConfig(t, `
server:
  listen: "0.0.0.0:8080"
  public_url: "https://wiki.example.local"
storage:
  data_dir: "/tmp/okf-data"
  repo_dir: "/tmp/okf-data/repo"
  max_upload_mb: 50
git:
  enabled: true
  remote_enabled: true
  remote: "origin"
  branch: "trunk"
  push_on_commit: true
  pull_on_startup: true
auth:
  session_cookie_name: "okf_session"
  session_ttl_hours: 168
agent:
  enabled: true
  provider: "openai-compatible"
  model: "qwen2.5"
  base_url: "http://localhost:11434/v1"
  api_key_env: "OKF_TEST_LLM_KEY"
search:
  enabled: true
  engine: "bleve"
attachments:
  extract_text: true
  allowed_extensions:
    - ".pdf"
    - ".docx"
`)
	t.Setenv("OKF_TEST_LLM_KEY", "super-secret-key-value")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Storage.MaxUploadMB != 50 {
		t.Errorf("max_upload_mb = %d, want 50", cfg.Storage.MaxUploadMB)
	}
	if !cfg.Git.RemoteEnabled || cfg.Git.Branch != "trunk" || !cfg.Git.PushOnCommit || !cfg.Git.PullOnStartup {
		t.Errorf("git section not parsed: %+v", cfg.Git)
	}
	if !cfg.Agent.Enabled || cfg.Agent.Model != "qwen2.5" || cfg.Agent.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("agent section not parsed: %+v", cfg.Agent)
	}
	if !cfg.Search.Enabled || cfg.Search.Engine != "bleve" {
		t.Errorf("search section not parsed: %+v", cfg.Search)
	}
	if !cfg.Attachments.ExtractText || len(cfg.Attachments.AllowedExtensions) != 2 {
		t.Errorf("attachments section not parsed: %+v", cfg.Attachments)
	}
	// api_key_env resolves the named env var into the (non-logged) API key.
	if got := cfg.Agent.APIKey(); got != "super-secret-key-value" {
		t.Errorf("resolved api key = %q, want the env value", got)
	}
}

func TestAPIKeyNeverInStringOrLog(t *testing.T) {
	p := writeTempConfig(t, `
server:
  listen: "0.0.0.0:8080"
storage:
  data_dir: "/tmp/okf-data"
agent:
  enabled: true
  api_key_env: "OKF_TEST_LLM_KEY2"
`)
	t.Setenv("OKF_TEST_LLM_KEY2", "do-not-leak-me")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The struct's %+v / %v / GoString rendering must NOT contain the secret —
	// a logged Config must never leak the resolved key.
	for _, rendered := range []string{
		fmt.Sprintf("%v", cfg),
		fmt.Sprintf("%+v", cfg),
		fmt.Sprintf("%#v", cfg),
		fmt.Sprintf("%v", cfg.Agent),
		fmt.Sprintf("%+v", cfg.Agent),
	} {
		if strings.Contains(rendered, "do-not-leak-me") {
			t.Errorf("rendered config leaked the resolved API key: %s", rendered)
		}
	}
	// But the key is still retrievable through the explicit accessor.
	if cfg.Agent.APIKey() != "do-not-leak-me" {
		t.Errorf("APIKey() = %q, want the resolved value", cfg.Agent.APIKey())
	}
}

func TestEnvOverridesDataDirAndListen(t *testing.T) {
	p := writeTempConfig(t, `
server:
  listen: "0.0.0.0:8080"
storage:
  data_dir: "/tmp/file-data"
`)
	t.Setenv(EnvDataDir, "/srv/okf")
	t.Setenv(EnvListen, "127.0.0.1:9999")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Storage.DataDir != "/srv/okf" {
		t.Errorf("data_dir = %q, want /srv/okf (env override)", cfg.Storage.DataDir)
	}
	if cfg.Server.Listen != "127.0.0.1:9999" {
		t.Errorf("listen = %q, want 127.0.0.1:9999 (env override)", cfg.Server.Listen)
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

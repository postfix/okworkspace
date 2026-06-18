// Package config loads and validates the OKF Workspace configuration
// (config.yaml per SPEC §20.3) into a typed Config struct, applying
// sensible defaults and a small set of environment overrides.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Defaults applied when optional keys are omitted.
const (
	DefaultSessionCookieName = "okf_session"
	DefaultSessionTTLHours   = 168 // 7 days
	DefaultAdminUsername     = "admin"
)

// Environment overrides for deployment (systemd/Docker). Each takes precedence
// over the value parsed from config.yaml when set to a non-empty string.
const (
	// EnvAdminUsername overrides admin.username (D-03).
	EnvAdminUsername = "OKF_ADMIN_USERNAME"
	// EnvDataDir overrides storage.data_dir (the container/systemd data volume).
	EnvDataDir = "OKF_DATA_DIR"
	// EnvListen overrides server.listen (the bind address/port).
	EnvListen = "OKF_LISTEN"
)

// Config mirrors config.yaml (SPEC §20.3). Agent/search/attachments keys are
// parsed-but-unused placeholders this phase so a real config.yaml round-trips
// without "unknown field" surprises.
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Storage     StorageConfig     `yaml:"storage"`
	Git         GitConfig         `yaml:"git"`
	Auth        AuthConfig        `yaml:"auth"`
	Admin       AdminConfig       `yaml:"admin"`
	Agent       AgentConfig       `yaml:"agent"`
	Search      SearchConfig      `yaml:"search"`
	Attachments AttachmentsConfig `yaml:"attachments"`
}

type ServerConfig struct {
	Listen    string `yaml:"listen"`
	PublicURL string `yaml:"public_url"`
}

type StorageConfig struct {
	DataDir     string `yaml:"data_dir"`
	RepoDir     string `yaml:"repo_dir"`
	MaxUploadMB int    `yaml:"max_upload_mb"`
}

type GitConfig struct {
	Enabled       bool   `yaml:"enabled"`
	RemoteEnabled bool   `yaml:"remote_enabled"`
	Remote        string `yaml:"remote"`
	Branch        string `yaml:"branch"`
	PushOnCommit  bool   `yaml:"push_on_commit"`
	PullOnStartup bool   `yaml:"pull_on_startup"`
}

type AuthConfig struct {
	SessionCookieName string `yaml:"session_cookie_name"`
	SessionTTLHours   int    `yaml:"session_ttl_hours"`
}

// AdminConfig configures the bootstrap admin account (D-03).
type AdminConfig struct {
	Username string `yaml:"username"`
}

// AgentConfig is parsed-but-unused in Phase 0 (placeholder for Phase 4). The
// resolved LLM API key is stored in an UNEXPORTED field and accessed only via
// APIKey(), and the type implements fmt.Stringer/fmt.GoStringer to REDACT the
// key — so a logged Config (%v / %+v / %#v) can never leak the secret
// (T-00.04-02, T-00.04-05).
type AgentConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`

	// apiKey is the value resolved from the APIKeyEnv environment variable at
	// Load time. It is never serialized (no yaml tag), never printed, and only
	// readable through APIKey().
	apiKey string `yaml:"-"`
}

// APIKey returns the LLM API key resolved from the api_key_env environment
// variable. It is the ONLY way to read the secret; it is never logged.
func (a AgentConfig) APIKey() string { return a.apiKey }

// String redacts the resolved API key so the agent config is safe to log.
func (a AgentConfig) String() string {
	return fmt.Sprintf("AgentConfig{Enabled:%t Provider:%q Model:%q BaseURL:%q APIKeyEnv:%q APIKey:[REDACTED]}",
		a.Enabled, a.Provider, a.Model, a.BaseURL, a.APIKeyEnv)
}

// GoString redacts the resolved API key under the %#v verb.
func (a AgentConfig) GoString() string { return a.String() }

// SearchConfig is parsed-but-unused in Phase 0 (placeholder for Phase 3).
type SearchConfig struct {
	Enabled bool   `yaml:"enabled"`
	Engine  string `yaml:"engine"`
}

// AttachmentsConfig is parsed-but-unused in Phase 0 (placeholder for Phase 2).
type AttachmentsConfig struct {
	ExtractText       bool     `yaml:"extract_text"`
	AllowedExtensions []string `yaml:"allowed_extensions"`
}

// Load reads the YAML config at path, applies defaults for omitted optional
// keys, and applies environment overrides.
func Load(path string) (Config, error) {
	var cfg Config
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	cfg.applyDefaults()
	cfg.applyEnvOverrides()
	cfg.resolveSecrets()
	return cfg, nil
}

// resolveSecrets reads the LLM API key from the named environment variable
// (agent.api_key_env, e.g. OKF_LLM_API_KEY) into the unexported apiKey field.
// The key is supplied at runtime via the environment (never baked into the
// image, T-00.04-05) and is never logged.
func (c *Config) resolveSecrets() {
	if c.Agent.APIKeyEnv != "" {
		c.Agent.apiKey = os.Getenv(c.Agent.APIKeyEnv)
	}
}

func (c *Config) applyDefaults() {
	if c.Auth.SessionCookieName == "" {
		c.Auth.SessionCookieName = DefaultSessionCookieName
	}
	if c.Auth.SessionTTLHours == 0 {
		c.Auth.SessionTTLHours = DefaultSessionTTLHours
	}
	if c.Admin.Username == "" {
		c.Admin.Username = DefaultAdminUsername
	}
}

func (c *Config) applyEnvOverrides() {
	if v := os.Getenv(EnvAdminUsername); v != "" {
		c.Admin.Username = v
	}
	if v := os.Getenv(EnvDataDir); v != "" {
		c.Storage.DataDir = v
	}
	if v := os.Getenv(EnvListen); v != "" {
		c.Server.Listen = v
	}
}

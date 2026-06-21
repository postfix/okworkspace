package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"

	"github.com/postfix/okworkspace/internal/config"
)

// deepseekConfig builds the live DeepSeek agent config from the environment.
// It writes a minimal config file and runs it through config.Load so the API
// key flows through the EXACT real seam the app uses (api_key_env → unexported
// apiKey → APIKey()). The raw key is never read or echoed by the test — only
// its presence is checked, via the env, to decide skip-vs-run.
func deepseekConfig(t *testing.T) config.AgentConfig {
	t.Helper()
	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		t.Skip("DEEPSEEK_API_KEY not set — skipping live DeepSeek smoke test (deterministic CI stays green key-free)")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	const body = `agent:
  enabled: true
  provider: "openai-compatible"
  model: "deepseek-v4-flash"
  base_url: "https://api.deepseek.com/v1"
  api_key_env: "DEEPSEEK_API_KEY"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg.Agent
}

// smokeIn / smokeOut are a tiny typed in/out pair used to prove utils.InferTool
// can derive a JSON schema from struct tags against the installed eino v0.9.9.
type smokeIn struct {
	Query string `json:"query" jsonschema:"description=the lookup query"`
}
type smokeOut struct {
	Answer string `json:"answer"`
}

// TestSmokeChatModelGenerate proves the configured DeepSeek model answers a
// single-shot Generate. It SKIPS cleanly when DEEPSEEK_API_KEY is unset so the
// deterministic suite stays green key-free; when the key is present it must
// reach the provider and return non-empty Content within 60s.
func TestSmokeChatModelGenerate(t *testing.T) {
	cfg := deepseekConfig(t)

	svc := NewService(cfg, nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.cm == nil {
		t.Fatal("enabled agent service has a nil ChatModel")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	out, err := svc.cm.Generate(ctx, []*schema.Message{
		schema.SystemMessage("You are a terse test probe. Reply with exactly one word."),
		schema.UserMessage("reply with the single word OK"),
	})
	if err != nil {
		// Network/quota failures are non-code failures: skip with a clear
		// message rather than failing the build (per plan critical note). A
		// genuine wiring/model-id error still surfaces here as a skip with the
		// provider's reason, which is visible in `go test -v`.
		if isProviderUnreachable(err) {
			t.Skipf("DeepSeek unreachable (non-code failure): %v", err)
		}
		t.Fatalf("single-shot Generate failed: %v", err)
	}
	if strings.TrimSpace(out.Content) == "" {
		t.Fatal("DeepSeek returned empty Content for a single-shot Generate")
	}
	t.Logf("DeepSeek single-shot Generate returned %d chars", len(out.Content))
}

// TestSmokeInferToolSchema proves the InferTool schema-derivation path works
// against the installed eino v0.9.9: a typed in/out struct yields a non-nil
// InvokableTool whose Info().Name equals the requested name. Key-free — always
// runs.
func TestSmokeInferToolSchema(t *testing.T) {
	const wantName = "smoke_lookup"
	tl, err := utils.InferTool(wantName, "A smoke-test lookup tool.",
		func(ctx context.Context, in smokeIn) (smokeOut, error) {
			return smokeOut{Answer: in.Query}, nil
		})
	if err != nil {
		t.Fatalf("InferTool failed: %v", err)
	}
	if tl == nil {
		t.Fatal("InferTool returned a nil tool")
	}
	info, err := tl.Info(context.Background())
	if err != nil {
		t.Fatalf("tool.Info failed: %v", err)
	}
	if info.Name != wantName {
		t.Fatalf("tool name = %q, want %q", info.Name, wantName)
	}
}

// isProviderUnreachable classifies network/transport/quota errors (non-code
// failures) so the live smoke test can skip rather than fail the build when the
// provider is simply unreachable. The error string is provider-supplied and
// never contains the API key (the key is sent in a header, not echoed back).
func isProviderUnreachable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, frag := range []string{
		"timeout", "deadline exceeded", "connection refused", "no such host",
		"dial tcp", "eof", "network is unreachable", "tls", "i/o timeout",
		"too many requests", "rate limit", "503", "502", "504",
	} {
		if strings.Contains(s, frag) {
			return true
		}
	}
	return false
}

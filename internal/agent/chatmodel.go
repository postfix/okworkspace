// Package agent wires the CloudWeGo Eino LLM agent into the workspace. This
// file builds the OpenAI-compatible ChatModel (which drives DeepSeek via its
// BaseURL) from the operator-controlled config.AgentConfig.
//
// Security invariant (T-04-01 / AGNT-11): the resolved API key is read ONLY via
// cfg.APIKey() and is NEVER logged, returned, or placed in an error string. The
// redacted AgentConfig Stringer (internal/config/config.go) keeps any logged
// config safe; this file must never widen that surface.
package agent

import (
	"context"

	"github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/postfix/okworkspace/internal/config"
)

// Default model sampling for the agent. MaxTokens is ALWAYS set in production
// so a runaway provider response can't be unbounded (T-04-02); Temperature is
// kept low for grounded answers.
const (
	defaultTemperature float32 = 0.2
	defaultMaxTokens   int     = 1024
)

// newChatModel builds the OpenAI-compatible ChatModel from the agent config.
// BaseURL + Model are operator-controlled; APIKey is the only secret read path
// (cfg.APIKey()) and is never logged. Temperature and MaxTokens are passed as
// pointers because the eino-ext ChatModelConfig treats nil as "provider
// default" — we always set MaxTokens in prod.
func newChatModel(ctx context.Context, cfg config.AgentConfig) (*openai.ChatModel, error) {
	temperature := defaultTemperature
	maxTokens := defaultMaxTokens
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL:     cfg.BaseURL,
		APIKey:      cfg.APIKey(), // ONLY secret accessor — never log this value.
		Model:       cfg.Model,
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
	})
}

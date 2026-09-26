// Package claude implements planner.LLM with the Anthropic API.
package claude

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"github.com/jalet/matlistan/internal/planner"
)

// DefaultModel is used unless MATLISTAN_MODEL says otherwise.
const DefaultModel = "claude-opus-5"

// maxTokens leaves room for adaptive thinking plus seven full recipes; streaming keeps the
// long request inside HTTP limits.
const maxTokens = 64000

// _fallbackModels support the server-side refusal fallback.
var _fallbackModels = map[string]bool{"claude-opus-5": true, "claude-fable-5-1": true}

// Config configures the client. BaseURL is for tests.
type Config struct {
	APIKey, Model, BaseURL string
	Timeout                time.Duration
}

// Client starts planning sessions.
type Client struct {
	api   anthropic.Client
	model string
}

// New builds a client with bounded retries and a per-request timeout.
func New(c Config) *Client {
	opts := []option.RequestOption{option.WithAPIKey(c.APIKey),
		option.WithRequestTimeout(c.Timeout), option.WithMaxRetries(2)}
	if c.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(c.BaseURL))
	}
	return &Client{api: anthropic.NewClient(opts...), model: c.Model}
}

// NewSession implements planner.LLM.
func (c *Client) NewSession(system string, schema map[string]any) planner.Session {
	return &session{c: c, system: system, schema: schema}
}

type session struct {
	c       *Client
	system  string
	schema  map[string]any
	history []anthropic.BetaMessageParam
}

// Send appends text as a user turn, streams the reply and keeps it in the history.
func (s *session) Send(ctx context.Context, text string) (planner.Reply, error) {
	s.history = append(s.history, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(text)))
	params := anthropic.BetaMessageNewParams{
		Model:     s.c.model,
		MaxTokens: maxTokens,
		System: []anthropic.BetaTextBlockParam{{Text: s.system,
			CacheControl: anthropic.NewBetaCacheControlEphemeralParam()}},
		Messages: s.history,
		OutputConfig: anthropic.BetaOutputConfigParam{
			Format: anthropic.BetaJSONOutputFormatParam{Schema: s.schema},
		},
	}
	if _fallbackModels[s.c.model] {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamUnion{
			OfDefault: constant.ValueOf[constant.Default](),
		}
	}
	stream := s.c.api.Beta.Messages.NewStreaming(ctx, params)
	msg := anthropic.BetaMessage{}
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return planner.Reply{}, fmt.Errorf("claude: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return planner.Reply{}, fmt.Errorf("claude: %w", err)
	}
	s.history = append(s.history, msg.ToParam())
	u := planner.Usage{Input: msg.Usage.InputTokens, Output: msg.Usage.OutputTokens,
		CacheRead: msg.Usage.CacheReadInputTokens, CacheWrite: msg.Usage.CacheCreationInputTokens}
	switch msg.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return planner.Reply{Usage: u}, planner.ErrRefused
	case anthropic.BetaStopReasonMaxTokens:
		return planner.Reply{Usage: u}, planner.ErrTruncated
	}
	var b strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			b.WriteString(t.Text)
		}
	}
	return planner.Reply{Text: b.String(), Usage: u}, nil
}

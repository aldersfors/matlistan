// Package openai plans weeks through the Chat Completions API of OpenAI or any
// OpenAI-compatible server, with the reply constrained to the planner's JSON schema.
package openai

import (
	"context"
	"errors"
	"fmt"
	"time"

	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"github.com/aldersfors/matlistan/internal/planner"
)

// maxTokens matches the Claude client: room for reasoning plus seven full recipes.
const maxTokens = 64000

// Config configures the client. BaseURL is required; APIKey may be empty for a local
// server, and then no Authorization header is sent.
type Config struct {
	APIKey, Model, BaseURL string
	Timeout                time.Duration
}

// Client starts planning sessions.
type Client struct {
	api   oai.Client
	model string
}

// New builds a client. The base URL and key are always passed explicitly: they override
// the OPENAI_* environment the SDK would otherwise read.
func New(c Config) *Client {
	return &Client{model: c.Model, api: oai.NewClient(option.WithBaseURL(c.BaseURL),
		option.WithAPIKey(c.APIKey), option.WithRequestTimeout(c.Timeout),
		option.WithMaxRetries(2))}
}

// NewSession implements planner.LLM.
func (c *Client) NewSession(system string, schema map[string]any) planner.Session {
	return &session{c: c, schema: schema,
		history: []oai.ChatCompletionMessageParamUnion{oai.SystemMessage(system)}}
}

type session struct {
	c       *Client
	schema  map[string]any
	history []oai.ChatCompletionMessageParamUnion
}

// Send appends text as a user turn and keeps the reply in the history.
func (s *session) Send(ctx context.Context, text string) (planner.Reply, error) {
	s.history = append(s.history, oai.UserMessage(text))
	resp, err := s.c.api.Chat.Completions.New(ctx, oai.ChatCompletionNewParams{
		Model:               shared.ChatModel(s.c.model),
		Messages:            s.history,
		MaxCompletionTokens: oai.Int(maxTokens),
		ResponseFormat: oai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name: "week_plan", Strict: oai.Bool(true), Schema: s.schema}}},
	})
	if err != nil {
		return planner.Reply{}, fmt.Errorf("openai: %w", err)
	}
	u := planner.Usage{Input: resp.Usage.PromptTokens, Output: resp.Usage.CompletionTokens,
		CacheRead: resp.Usage.PromptTokensDetails.CachedTokens}
	if len(resp.Choices) == 0 {
		return planner.Reply{Usage: u}, errors.New("openai: empty reply")
	}
	ch := resp.Choices[0]
	// Keep any answer in the history before judging it: the planner's follow-up after a
	// cut-off answer must alternate roles, which strict chat templates enforce.
	if ch.Message.Content != "" {
		s.history = append(s.history, oai.AssistantMessage(ch.Message.Content))
	}
	switch {
	case ch.Message.Refusal != "":
		return planner.Reply{Usage: u}, planner.ErrRefused
	case ch.FinishReason == "length":
		return planner.Reply{Usage: u}, planner.ErrTruncated
	case ch.Message.Content == "":
		return planner.Reply{Usage: u}, errors.New("openai: empty reply")
	}
	return planner.Reply{Text: ch.Message.Content, Usage: u}, nil
}

package planner

import (
	"context"
	"errors"
)

// Usage counts the tokens of one reply.
type Usage struct{ Input, Output, CacheRead, CacheWrite int64 }

// Reply is the model's text and what it cost.
type Reply struct {
	Text  string
	Usage Usage
}

// Session is one conversation: the retry continues it with feedback.
type Session interface {
	Send(ctx context.Context, text string) (Reply, error)
}

// LLM starts sessions with a fixed system prompt and output schema.
type LLM interface {
	NewSession(system string, schema map[string]any) Session
}

// Reply errors that are not transport errors.
var (
	ErrRefused   = errors.New("the model declined the request")
	ErrTruncated = errors.New("the answer was cut off at the token limit")
)

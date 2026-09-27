package main

import (
	"testing"

	"github.com/aldersfors/matlistan/internal/config"
	"github.com/aldersfors/matlistan/internal/planner/claude"
	"github.com/aldersfors/matlistan/internal/planner/openai"
)

func TestNewLLMPicksTheProvider(t *testing.T) {
	if _, ok := newLLM(config.Planner{Provider: config.ProviderAnthropic, Model: "claude-opus-5"},
		"k").(*claude.Client); !ok {
		t.Error("anthropic did not build the Claude client")
	}
	if _, ok := newLLM(config.Planner{Provider: config.ProviderOpenAI, Model: "gpt-6"},
		"k").(*openai.Client); !ok {
		t.Error("openai did not build the OpenAI client")
	}
}

func TestWebProvider(t *testing.T) {
	cases := map[string]struct {
		p          config.Planner
		name, host string
	}{
		"anthropic": {config.Planner{Provider: "anthropic"}, "anthropic", ""},
		"openai":    {config.Planner{Provider: "openai"}, "openai", ""},
		"local": {config.Planner{Provider: "openai", BaseURL: "http://ollama.ml.svc:11434/v1"},
			"openai", "ollama.ml.svc"},
	}
	for n, c := range cases {
		got := webProvider(c.p)
		if got.Name != c.name || got.Host != c.host {
			t.Errorf("%s: %+v", n, got)
		}
	}
}

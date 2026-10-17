package cli

import "testing"

func TestObservedModelRequiresCLIHeader(t *testing.T) {
	if observedModel("ordinary response") != "" {
		t.Fatal("requested model was fabricated as observed")
	}
	if observedModel("OpenAI Codex\nmodel: gpt-6.1-sol\nprovider: openai\n") != "gpt-6.1-sol" {
		t.Fatal("model header not parsed")
	}
}

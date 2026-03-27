package agent

import (
	"strings"
	"testing"

	"csgclaw/internal/config"
)

func TestRenderManagerSecurityConfig(t *testing.T) {
	got := renderManagerSecurityConfig(config.LLMConfig{
		ModelID: "minimax-m2.7",
		APIKey:  "sk-1234567890",
	})

	for _, want := range []string{
		"model_list:\n",
		"  minimax-m2.7:0:\n",
		"    api_keys:\n",
		"      - sk-1234567890\n",
		"channels: {}\n",
		"web: {}\n",
		"skills: {}\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderManagerSecurityConfig() missing %q in:\n%s", want, got)
		}
	}
}

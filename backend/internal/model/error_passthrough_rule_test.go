package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllPlatformsIncludesEveryConcretePlatform(t *testing.T) {
	require.ElementsMatch(t, []string{
		"anthropic",
		"openai",
		"gemini",
		"antigravity",
		"grok",
		"kimi",
		"zhipu",
		"deepseek",
		"minimax",
		"opencode_go",
		"typesafe",
		"ag_hub", // FORK-ANCHOR: ag-hub-platform-registry (二开：聚合中转登记进平台清单)
		"command_code",
		"cline",
	}, AllPlatforms())
}

package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwsBedrockGlobalClaudeRatios(t *testing.T) {
	InitRatioSettings()

	tests := []struct {
		model string
		ratio float64
	}{
		{"claude-opus-4-6-v1", 2.5},
		{"global.anthropic.claude-sonnet-4-6", 1.5},
		{"global.anthropic.claude-haiku-4-5-20251001-v1:0", 0.5},
		{"global.anthropic.claude-opus-4-6-v1", 2.5},
		{"global.anthropic.claude-opus-4-7", 2.5},
		{"global.anthropic.claude-opus-4-8", 2.5},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			modelRatio, ok, matchName := GetModelRatio(test.model)
			require.True(t, ok)
			assert.Equal(t, test.model, matchName)
			assert.Equal(t, test.ratio, modelRatio)

			cacheRatio, ok := GetCacheRatio(test.model)
			require.True(t, ok)
			assert.Equal(t, 0.1, cacheRatio)

			createCacheRatio, ok := GetCreateCacheRatio(test.model)
			require.True(t, ok)
			assert.Equal(t, 1.25, createCacheRatio)
		})
	}
}

func TestAwsBedrockClaudeFableRatios(t *testing.T) {
	InitRatioSettings()

	tests := []struct {
		model string
		ratio float64
	}{
		{"claude-fable-5", 5.5},
		{"anthropic.claude-fable-5", 5.5},
		{"us.anthropic.claude-fable-5", 5.5},
		{"eu.anthropic.claude-fable-5", 5.5},
		{"global.anthropic.claude-fable-5", 5},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			modelRatio, ok, matchName := GetModelRatio(test.model)
			require.True(t, ok)
			assert.Equal(t, test.model, matchName)
			assert.Equal(t, test.ratio, modelRatio)
			assert.Equal(t, 5.0, GetCompletionRatio(test.model))

			cacheRatio, ok := GetCacheRatio(test.model)
			require.True(t, ok)
			assert.Equal(t, 0.1, cacheRatio)

			createCacheRatio, ok := GetCreateCacheRatio(test.model)
			require.True(t, ok)
			assert.Equal(t, 1.25, createCacheRatio)
		})
	}
}

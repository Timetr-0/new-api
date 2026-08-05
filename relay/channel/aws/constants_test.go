package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeFableAwsModelIDAndCrossRegion(t *testing.T) {
	modelID := getAwsModelID("claude-fable-5")
	require.Equal(t, "anthropic.claude-fable-5", modelID)

	assert.True(t, awsModelCanCrossRegion(modelID, "us"))
	assert.True(t, awsModelCanCrossRegion(modelID, "eu"))
	assert.False(t, awsModelCanCrossRegion(modelID, "ap"))

	assert.Equal(t, "us.anthropic.claude-fable-5", awsModelCrossRegion(modelID, "us"))
	assert.Equal(t, "eu.anthropic.claude-fable-5", awsModelCrossRegion(modelID, "eu"))
}

func TestClaude5AwsModelIDAndCrossRegion(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
	}{
		{"claude-sonnet-5", "anthropic.claude-sonnet-5"},
		{"claude-opus-5", "anthropic.claude-opus-5"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelID := getAwsModelID(test.name)
			require.Equal(t, test.modelID, modelID)

			assert.True(t, awsModelCanCrossRegion(modelID, "us"))
			assert.True(t, awsModelCanCrossRegion(modelID, "eu"))
			assert.False(t, awsModelCanCrossRegion(modelID, "ap"))

			assert.Equal(t, "us."+test.modelID, awsModelCrossRegion(modelID, "us"))
			assert.Equal(t, "eu."+test.modelID, awsModelCrossRegion(modelID, "eu"))
		})
	}
}

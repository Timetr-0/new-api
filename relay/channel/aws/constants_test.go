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

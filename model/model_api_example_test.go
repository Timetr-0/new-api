package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelAPIExamplesPersistByModelEndpointAndLanguage(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ModelAPIExample{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&ModelAPIExample{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Where("1 = 1").Delete(&ModelAPIExample{}).Error)
	})

	for _, example := range []ModelAPIExample{
		{ModelName: "model-a", EndpointType: "openai", Language: "curl", Code: "curl original\n"},
		{ModelName: "model-a", EndpointType: "openai", Language: "python", Code: "print('python')"},
		{ModelName: "model-a", EndpointType: "anthropic", Language: "curl", Code: "curl messages"},
		{ModelName: "model-b", EndpointType: "openai", Language: "curl", Code: "curl model-b"},
	} {
		require.NoError(t, UpdateModelAPIExample(example))
	}
	require.NoError(t, UpdateModelAPIExample(ModelAPIExample{
		ModelName: "model-a", EndpointType: "openai", Language: "curl", Code: "curl edited\n",
	}))

	examples, err := GetModelAPIExamples("model-a")
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{
		"openai":    {"curl": "curl edited\n", "python": "print('python')"},
		"anthropic": {"curl": "curl messages"},
	}, examples)

	require.NoError(t, UpdateModelAPIExample(ModelAPIExample{
		ModelName: "model-a", EndpointType: "openai", Language: "curl", Code: " \n",
	}))
	examples, err = GetModelAPIExamples("model-a")
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{
		"openai":    {"python": "print('python')"},
		"anthropic": {"curl": "curl messages"},
	}, examples)
	examples, err = GetModelAPIExamples("model-b")
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{"openai": {"curl": "curl model-b"}}, examples)
	examples, err = GetModelAPIExamples("unknown-model")
	require.NoError(t, err)
	assert.Empty(t, examples)
}

func TestModelAPIExampleRejectsInvalidInput(t *testing.T) {
	for _, example := range []ModelAPIExample{
		{ModelName: "", EndpointType: "openai", Language: "curl"},
		{ModelName: "model", EndpointType: "", Language: "curl"},
		{ModelName: "model", EndpointType: "openai", Language: "shell"},
		{ModelName: "model", EndpointType: "openai", Language: "curl", Code: strings.Repeat("x", 60*1024+1)},
	} {
		assert.Error(t, UpdateModelAPIExample(example))
	}
}

package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRequestInterceptionRulesJSON(t *testing.T) {
	require.NoError(t, ValidateRequestInterceptionRulesJSON(`[]`))
	require.NoError(t, ValidateRequestInterceptionRulesJSON(`[
		{
			"name": "test",
			"model_regex": ["^claude-.*"],
			"conditions": [{"path": "thinking.type", "operator": "exists"}],
			"error": {"status_code": 400, "message": "blocked"}
		}
	]`))

	assert.ErrorContains(t, ValidateRequestInterceptionRulesJSON(`null`), "rules must be a JSON array")
	assert.ErrorContains(t, ValidateRequestInterceptionRulesJSON(`[
		{
			"name": "bad regex",
			"model_regex": ["["],
			"conditions": [{"path": "thinking.type", "operator": "exists"}],
			"error": {"status_code": 400, "message": "blocked"}
		}
	]`), "invalid model_regex")
}

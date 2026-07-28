package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrainingDataRecordPathUsesSixHourClaudeWindows(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("CST", 8*60*60)
	defer func() {
		time.Local = oldLocal
	}()

	cases := []struct {
		hour int
		want string
	}{
		{0, "/data/training-capture/2026-07-28/claude-00-05.jsonl"},
		{5, "/data/training-capture/2026-07-28/claude-00-05.jsonl"},
		{6, "/data/training-capture/2026-07-28/claude-06-11.jsonl"},
		{12, "/data/training-capture/2026-07-28/claude-12-17.jsonl"},
		{23, "/data/training-capture/2026-07-28/claude-18-23.jsonl"},
	}

	for _, tc := range cases {
		got := trainingDataRecordPath(time.Date(2026, 7, 28, tc.hour, 30, 0, 0, time.Local))
		require.Equal(t, tc.want, got)
	}
}

func TestBuildTrainingDataRecordEntryKeepsOnlyOriginalPayloadsWithoutUserMeta(t *testing.T) {
	state := &trainingDataRecordState{
		requestBody: []byte(`{
			"model": "claude-sonnet-4-5",
			"messages": [{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "plan", "signature": "sig-123"},
					{"type": "redacted_thinking", "data": "encrypted", "signature": "sig-456"},
					{"type": "tool_result", "content": "ok"}
				]
			}],
			"tools": [{"name": "search"}],
			"tool_choice": {"type": "any"}
		}`),
		responseBody: []byte(`{
			"id": "msg_1",
			"type": "message",
			"model": "claude-sonnet-4-5",
			"content": [{"type": "tool_use", "name": "search", "input": {}}],
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`),
	}

	entry := buildTrainingDataRecordEntry(state)

	line, err := common.Marshal(entry)
	require.NoError(t, err)
	record := string(line)

	assert.Contains(t, record, `"signature":"sig-123"`)
	assert.Contains(t, record, `"redacted_thinking"`)
	assert.Contains(t, record, `"signature":"sig-456"`)
	assert.Contains(t, record, `"thinking":"plan"`)
	assert.Contains(t, record, `"request":`)
	assert.Contains(t, record, `"response":`)
	assert.NotContains(t, record, "schema_version")
	assert.NotContains(t, record, "captured_at")
	assert.NotContains(t, record, "request_path")
	assert.NotContains(t, record, "method")
	assert.NotContains(t, record, "relay_format")
	assert.NotContains(t, record, "final_request_format")
	assert.NotContains(t, record, "channel_type")
	assert.NotContains(t, record, "requested_model")
	assert.NotContains(t, record, "upstream_model")
	assert.NotContains(t, record, "status_code")
	assert.NotContains(t, record, "stream")
	assert.NotContains(t, record, "tools_present")
	assert.NotContains(t, record, "tool_results_present")
	assert.NotContains(t, record, "tool_choice_type")
	assert.NotContains(t, record, "configured_tool_names")
	assert.NotContains(t, record, "response_tool_call_names")
	assert.NotContains(t, record, "usage_semantic")
	assert.NotContains(t, record, "user_id")
	assert.NotContains(t, record, "token_id")
	assert.NotContains(t, record, "token_key")
	assert.NotContains(t, record, "user@example.com")
	assert.NotContains(t, record, "sk-secret")
}

func TestAppendTrainingDataRecordEntriesWritesPlainJsonl(t *testing.T) {
	path := filepath.Join(t.TempDir(), "2026-07-28", "claude-00-05.jsonl")
	entry := trainingDataRecordEntry{
		capturedAt: time.Date(2026, 7, 28, 1, 0, 0, 0, time.Local),
		Request:    json.RawMessage(`{"model":"claude-sonnet-4-5"}`),
		Response:   json.RawMessage(`{"type":"message"}`),
	}

	require.NoError(t, appendTrainingDataRecordEntries(path, []trainingDataRecordEntry{entry}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	line := strings.TrimSpace(string(data))
	assert.Contains(t, line, `"request":{"model":"claude-sonnet-4-5"}`)
	assert.Contains(t, line, `"response":{"type":"message"}`)
}

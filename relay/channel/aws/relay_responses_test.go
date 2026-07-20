package aws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAWSConvertOpenAIResponsesRequestToClaudeMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	maxOutputTokens := uint(128)
	stream := true
	info := newAWSResponsesRelayInfo(true)
	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
		Model:           "claude-opus-4-6",
		Input:           []byte(`"hello"`),
		MaxOutputTokens: &maxOutputTokens,
		Stream:          &stream,
	})

	require.NoError(t, err)
	claudeReq, ok := converted.(*dto.ClaudeRequest)
	require.True(t, ok)
	require.Equal(t, "claude-opus-4-6", claudeReq.Model)
	require.NotNil(t, claudeReq.MaxTokens)
	assert.Equal(t, maxOutputTokens, *claudeReq.MaxTokens)
	require.NotNil(t, claudeReq.Stream)
	assert.True(t, *claudeReq.Stream)
	require.Len(t, claudeReq.Messages, 1)
	assert.Equal(t, "user", claudeReq.Messages[0].Role)
	content, err := claudeReq.Messages[0].ParseContent()
	require.NoError(t, err)
	require.Len(t, content, 1)
	assert.Equal(t, "hello", content[0].GetText())
	assert.Equal(t, "claude-opus-4-6", info.UpstreamModelName)
}

func TestAWSResponsesHandlerReturnsOpenAIResponsesJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "aws-responses-test")

	body := mustAWSClaudeResponseBody(t, dto.ClaudeResponse{
		Id:         "msg_1",
		Type:       "message",
		Role:       "assistant",
		Model:      "claude-opus-4-6",
		StopReason: "end_turn",
		Content: []dto.ClaudeMediaMessage{
			{
				Type: "text",
				Text: common.GetPointer("hello"),
			},
		},
		Usage: &dto.ClaudeUsage{
			InputTokens:  2,
			OutputTokens: 3,
		},
	})

	apiErr, usage := awsWriteResponsesBody(c, newAWSResponsesRelayInfo(false), body, nil)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 2, usage.PromptTokens)
	assert.Equal(t, 3, usage.CompletionTokens)
	assert.Equal(t, 5, usage.TotalTokens)

	got := recorder.Body.String()
	assert.Contains(t, got, `"object":"response"`)
	assert.Contains(t, got, `"status":"completed"`)
	assert.Contains(t, got, `"type":"output_text"`)
	assert.Contains(t, got, `"text":"hello"`)
	assert.Contains(t, got, `"input_tokens":2`)
	assert.Contains(t, got, `"output_tokens":3`)
	assert.NotContains(t, got, `"choices"`)
}

func TestAWSResponsesStreamConverterReturnsOpenAIResponsesSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "aws-responses-stream-test")

	info := newAWSResponsesRelayInfo(true)
	converter, apiErr := newAWSResponsesStreamConverter(c, info)
	require.Nil(t, apiErr)

	stopReason := "end_turn"
	chunks := []dto.ClaudeResponse{
		{
			Type: "message_start",
			Message: &dto.ClaudeMediaMessage{
				Id:    "msg_1",
				Type:  "message",
				Role:  "assistant",
				Model: "claude-opus-4-6",
				Usage: &dto.ClaudeUsage{
					InputTokens: 2,
				},
			},
		},
		{
			Type:  "content_block_delta",
			Index: common.GetPointer(0),
			Delta: &dto.ClaudeMediaMessage{
				Type: "text_delta",
				Text: common.GetPointer("hello"),
			},
		},
		{
			Type: "message_delta",
			Delta: &dto.ClaudeMediaMessage{
				StopReason: &stopReason,
			},
			Usage: &dto.ClaudeUsage{
				OutputTokens: 3,
			},
		},
		{Type: "message_stop"},
	}
	for _, chunk := range chunks {
		err := converter.HandleClaudeChunk(mustAWSClaudeResponseBody(t, chunk))
		require.Nil(t, err)
	}
	usage, apiErr := converter.Finalize()
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 2, usage.PromptTokens)
	assert.Equal(t, 3, usage.CompletionTokens)
	assert.Equal(t, 5, usage.TotalTokens)

	got := recorder.Body.String()
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	assert.Contains(t, got, `event: response.created`)
	assert.Contains(t, got, `event: response.output_text.delta`)
	assert.Contains(t, got, `"delta":"hello"`)
	assert.Contains(t, got, `event: response.completed`)
	assert.Contains(t, got, `"input_tokens":2`)
	assert.Contains(t, got, `"output_tokens":3`)
	assert.NotContains(t, got, `"choices"`)
	requireOrderedAWSResponsesSubstrings(t, got,
		`event: response.created`,
		`event: response.output_item.added`,
		`event: response.output_text.delta`,
		`event: response.output_text.done`,
		`event: response.completed`,
	)
}

func newAWSResponsesRelayInfo(isStream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		IsStream:        isStream,
		RelayMode:       relayconstant.RelayModeResponses,
		RelayFormat:     types.RelayFormatOpenAIResponses,
		RequestURLPath:  "/v1/responses",
		DisablePing:     true,
		OriginModelName: "claude-opus-4-6",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "claude-opus-4-6",
		},
	}
}

func mustAWSClaudeResponseBody(t *testing.T, response dto.ClaudeResponse) []byte {
	t.Helper()
	body, err := common.Marshal(response)
	require.NoError(t, err)
	return body
}

func requireOrderedAWSResponsesSubstrings(t *testing.T, s string, parts ...string) {
	t.Helper()
	offset := 0
	for _, part := range parts {
		idx := strings.Index(s[offset:], part)
		require.NotEqualf(t, -1, idx, "missing %q after byte offset %d", part, offset)
		offset += idx + len(part)
	}
}

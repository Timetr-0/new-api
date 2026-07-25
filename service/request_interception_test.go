package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setRequestInterceptionSettingForTest(t *testing.T, setting operation_setting.RequestInterceptionSetting) {
	t.Helper()
	target := operation_setting.GetRequestInterceptionSetting()
	original := *target
	*target = setting
	t.Cleanup(func() {
		*target = original
	})
}

func buildRequestInterceptionContextForTest(channelType int, path string) (*gin.Context, *relaycommon.RelayInfo) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, path, nil)
	common.SetContextKey(ctx, constant.ContextKeyChannelType, channelType)
	common.SetContextKey(ctx, constant.ContextKeyChannelId, 213)
	common.SetContextKey(ctx, constant.ContextKeyChannelName, "aws-test-channel")
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "claude-sonnet-4-6")

	info := &relaycommon.RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		OriginModelName:        "claude-sonnet-4-6",
		RequestURLPath:         path,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       channelType,
			ChannelId:         213,
			UpstreamModelName: "claude-sonnet-4-6",
		},
	}
	return ctx, info
}

func TestCheckRequestInterceptionDefaultRuleRejectsAwsThinkingWithForcedToolChoice(t *testing.T) {
	setRequestInterceptionSettingForTest(t, operation_setting.RequestInterceptionSetting{
		Enabled: true,
		Rules:   operation_setting.DefaultRequestInterceptionRules(),
	})
	ctx, info := buildRequestInterceptionContextForTest(constant.ChannelTypeAws, "/v1/messages")

	err := CheckRequestInterception(ctx, info, []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [{"role": "user", "content": "hi"}],
		"thinking": {"type": "enabled", "budget_tokens": 1280},
		"tool_choice": {"type": "tool", "name": "lookup"}
	}`))

	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.True(t, types.IsSkipRetryError(err))
	assert.False(t, types.IsRecordErrorLog(err))

	openAIError := err.ToOpenAIError()
	assert.Equal(t, operation_setting.DefaultAwsThinkingForcedToolChoiceMessage, openAIError.Message)
	assert.Equal(t, "invalid_request_error", openAIError.Type)
	assert.Equal(t, string(types.ErrorCodeInvalidRequest), openAIError.Code)
}

func TestCheckRequestInterceptionDefaultRuleAllowsAutoToolChoice(t *testing.T) {
	setRequestInterceptionSettingForTest(t, operation_setting.RequestInterceptionSetting{
		Enabled: true,
		Rules:   operation_setting.DefaultRequestInterceptionRules(),
	})
	ctx, info := buildRequestInterceptionContextForTest(constant.ChannelTypeAws, "/v1/messages")

	err := CheckRequestInterception(ctx, info, []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [{"role": "user", "content": "hi"}],
		"thinking": {"type": "enabled", "budget_tokens": 1280},
		"tool_choice": {"type": "auto"}
	}`))

	require.Nil(t, err)
}

func TestCheckRequestInterceptionDefaultRuleIgnoresNonAwsChannels(t *testing.T) {
	setRequestInterceptionSettingForTest(t, operation_setting.RequestInterceptionSetting{
		Enabled: true,
		Rules:   operation_setting.DefaultRequestInterceptionRules(),
	})
	ctx, info := buildRequestInterceptionContextForTest(constant.ChannelTypeAnthropic, "/v1/messages")

	err := CheckRequestInterception(ctx, info, []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [{"role": "user", "content": "hi"}],
		"thinking": {"type": "enabled", "budget_tokens": 1280},
		"tool_choice": {"type": "tool", "name": "lookup"}
	}`))

	require.Nil(t, err)
}

func TestCheckRequestInterceptionCustomRuleReturnsConfiguredError(t *testing.T) {
	setRequestInterceptionSettingForTest(t, operation_setting.RequestInterceptionSetting{
		Enabled: true,
		Rules: []operation_setting.RequestInterceptionRule{
			{
				Name:         "metadata block",
				ChannelTypes: []int{constant.ChannelTypeAws},
				Conditions: []operation_setting.RequestInterceptionCondition{
					{Path: "metadata.block", Operator: "eq", Value: "yes"},
				},
				Error: operation_setting.RequestInterceptionError{
					StatusCode: http.StatusUnprocessableEntity,
					Type:       "invalid_request_error",
					Code:       "blocked_request",
					Message:    "blocked by local rule",
				},
			},
		},
	})
	ctx, info := buildRequestInterceptionContextForTest(constant.ChannelTypeAws, "/v1/messages")

	err := CheckRequestInterception(ctx, info, []byte(`{"metadata":{"block":"yes"}}`))

	require.NotNil(t, err)
	assert.Equal(t, http.StatusUnprocessableEntity, err.StatusCode)
	openAIError := err.ToOpenAIError()
	assert.Equal(t, "blocked by local rule", openAIError.Message)
	assert.Equal(t, "blocked_request", openAIError.Code)
	assert.True(t, types.IsSkipRetryError(err))
	assert.False(t, types.IsRecordErrorLog(err))
}

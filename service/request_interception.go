package service

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type requestInterceptionContext struct {
	ChannelType      int
	ChannelID        int
	ChannelName      string
	RelayFormat      string
	FinalRelayFormat string
	OriginModel      string
	UpstreamModel    string
	Path             string
	Method           string
}

func CheckRequestInterception(c *gin.Context, info *relaycommon.RelayInfo, requestBody []byte) *types.NewAPIError {
	setting := operation_setting.GetRequestInterceptionSetting()
	if setting == nil || !setting.Enabled || len(setting.Rules) == 0 {
		return nil
	}
	if len(requestBody) == 0 || !gjson.ValidBytes(requestBody) {
		return nil
	}

	ctx := buildRequestInterceptionContext(c, info)
	for _, rule := range setting.Rules {
		if !requestInterceptionRuleMatches(rule, ctx, requestBody) {
			continue
		}
		if c != nil {
			logger.LogWarn(c, fmt.Sprintf("request intercepted by rule %q: %s", rule.Name, rule.Error.Message))
		}
		newAPIError := newRequestInterceptionError(rule)
		recordRequestInterceptionWarningLog(c, info, ctx, rule, newAPIError)
		return newAPIError
	}
	return nil
}

func recordRequestInterceptionWarningLog(c *gin.Context, info *relaycommon.RelayInfo, ctx requestInterceptionContext, rule operation_setting.RequestInterceptionRule, newAPIError *types.NewAPIError) {
	if c == nil || model.LOG_DB == nil {
		return
	}

	userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if userId == 0 && info != nil {
		userId = info.UserId
	}
	if userId == 0 {
		return
	}
	tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	if tokenId == 0 && info != nil {
		tokenId = info.TokenId
	}
	tokenName := c.GetString("token_name")
	modelName := ctx.OriginModel
	if modelName == "" {
		modelName = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	}
	if modelName == "" && info != nil {
		modelName = info.OriginModelName
	}
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	}
	if group == "" && info != nil {
		group = info.UsingGroup
	}
	if group == "" && info != nil {
		group = info.TokenGroup
	}

	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	useTimeSeconds := int(time.Since(startTime).Seconds())
	content := newAPIError.MaskSensitiveErrorWithStatusCode()
	if content == "" {
		content = fmt.Sprintf("request intercepted by rule %q", rule.Name)
	}

	other := map[string]interface{}{
		"reject_reason":     content,
		"interception_rule": rule.Name,
		"error_type":        newAPIError.GetErrorType(),
		"error_code":        newAPIError.GetErrorCode(),
		"status_code":       newAPIError.StatusCode,
		"channel_id":        ctx.ChannelID,
		"channel_name":      ctx.ChannelName,
		"channel_type":      ctx.ChannelType,
	}
	if ctx.Path != "" {
		other["request_path"] = ctx.Path
	}
	conversionChain := make([]string, 0, 2)
	if ctx.RelayFormat != "" {
		conversionChain = append(conversionChain, ctx.RelayFormat)
	}
	if ctx.FinalRelayFormat != "" && ctx.FinalRelayFormat != ctx.RelayFormat {
		conversionChain = append(conversionChain, ctx.FinalRelayFormat)
	}
	if len(conversionChain) > 0 {
		other["request_conversion"] = conversionChain
	}

	adminInfo := map[string]interface{}{
		"use_channel": c.GetStringSlice("use_channel"),
	}
	if common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey) {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
	}
	AppendChannelAffinityAdminInfo(c, adminInfo)
	other["admin_info"] = adminInfo

	model.RecordWarningLog(c, userId, ctx.ChannelID, modelName, tokenName, content, tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), group, other)
}

func buildRequestInterceptionContext(c *gin.Context, info *relaycommon.RelayInfo) requestInterceptionContext {
	ctx := requestInterceptionContext{}
	if info != nil {
		ctx.ChannelType = info.ChannelType
		ctx.ChannelID = info.ChannelId
		ctx.RelayFormat = string(info.RelayFormat)
		ctx.FinalRelayFormat = string(info.GetFinalRequestRelayFormat())
		ctx.OriginModel = info.OriginModelName
		ctx.UpstreamModel = info.UpstreamModelName
		ctx.Path = info.RequestURLPath
	}
	if c == nil {
		return ctx
	}

	if ctx.ChannelType == 0 {
		ctx.ChannelType = common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	}
	if ctx.ChannelID == 0 {
		ctx.ChannelID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	}
	ctx.ChannelName = common.GetContextKeyString(c, constant.ContextKeyChannelName)
	if ctx.UpstreamModel == "" {
		ctx.UpstreamModel = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	}
	if ctx.Path == "" && c.Request != nil && c.Request.URL != nil {
		ctx.Path = c.Request.URL.Path
	}
	if c.Request != nil {
		ctx.Method = c.Request.Method
	}
	return ctx
}

func requestInterceptionRuleMatches(rule operation_setting.RequestInterceptionRule, ctx requestInterceptionContext, requestBody []byte) bool {
	if rule.Disabled {
		return false
	}
	if !matchInt(rule.ChannelTypes, ctx.ChannelType) {
		return false
	}
	if !matchInt(rule.ChannelIds, ctx.ChannelID) {
		return false
	}
	if !matchAnyRegex(rule.ChannelNameRegex, ctx.ChannelName) {
		return false
	}
	if !matchAnyStringFold(rule.RelayFormats, ctx.FinalRelayFormat, ctx.RelayFormat) {
		return false
	}
	if !matchAnyRegex(rule.ModelRegex, ctx.UpstreamModel, ctx.OriginModel) {
		return false
	}
	if !matchAnyRegex(rule.PathRegex, ctx.Path) {
		return false
	}
	if !matchAnyStringFold(rule.Methods, ctx.Method) {
		return false
	}
	return requestInterceptionConditionsMatch(rule, requestBody)
}

func matchInt(expected []int, actual int) bool {
	if len(expected) == 0 {
		return true
	}
	for _, value := range expected {
		if value == actual {
			return true
		}
	}
	return false
}

func matchAnyStringFold(expected []string, candidates ...string) bool {
	if len(expected) == 0 {
		return true
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		for _, value := range expected {
			if strings.EqualFold(strings.TrimSpace(value), candidate) {
				return true
			}
		}
	}
	return false
}

func matchAnyRegex(patterns []string, candidates ...string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		for _, pattern := range patterns {
			matched, err := regexp.MatchString(pattern, candidate)
			if err == nil && matched {
				return true
			}
		}
	}
	return false
}

func requestInterceptionConditionsMatch(rule operation_setting.RequestInterceptionRule, requestBody []byte) bool {
	for _, condition := range rule.Conditions {
		if !requestInterceptionConditionMatches(condition, requestBody) {
			return false
		}
	}
	if len(rule.AnyConditions) == 0 {
		return true
	}
	for _, condition := range rule.AnyConditions {
		if requestInterceptionConditionMatches(condition, requestBody) {
			return true
		}
	}
	return false
}

func requestInterceptionConditionMatches(condition operation_setting.RequestInterceptionCondition, requestBody []byte) bool {
	operator := strings.ToLower(strings.TrimSpace(condition.Operator))
	if operator == "" {
		operator = "exists"
	}

	result := gjson.GetBytes(requestBody, condition.Path)
	switch operator {
	case "exists":
		return result.Exists()
	case "not_exists":
		return !result.Exists()
	case "eq", "equals":
		return result.Exists() && requestInterceptionResultString(result) == requestInterceptionValueString(condition.Value)
	case "ne", "not_equals":
		return !result.Exists() || requestInterceptionResultString(result) != requestInterceptionValueString(condition.Value)
	case "in":
		return result.Exists() && requestInterceptionValueInList(requestInterceptionResultString(result), condition)
	case "not_in":
		return !result.Exists() || !requestInterceptionValueInList(requestInterceptionResultString(result), condition)
	case "regex":
		return result.Exists() && requestInterceptionRegexMatches(requestInterceptionValueString(condition.Value), requestInterceptionResultString(result))
	case "not_regex":
		return !result.Exists() || !requestInterceptionRegexMatches(requestInterceptionValueString(condition.Value), requestInterceptionResultString(result))
	case "contains":
		return result.Exists() && strings.Contains(requestInterceptionResultString(result), requestInterceptionValueString(condition.Value))
	case "not_contains":
		return !result.Exists() || !strings.Contains(requestInterceptionResultString(result), requestInterceptionValueString(condition.Value))
	case "gt", "gte", "lt", "lte":
		return result.Exists() && requestInterceptionNumberMatches(operator, result.Float(), condition.Value)
	default:
		return false
	}
}

func requestInterceptionResultString(result gjson.Result) string {
	if !result.Exists() {
		return ""
	}
	return result.String()
}

func requestInterceptionValueString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func requestInterceptionValueInList(actual string, condition operation_setting.RequestInterceptionCondition) bool {
	values := condition.Values
	if len(values) == 0 {
		switch v := condition.Value.(type) {
		case []any:
			values = v
		case []string:
			values = make([]any, 0, len(v))
			for _, item := range v {
				values = append(values, item)
			}
		default:
			if condition.Value != nil {
				values = []any{condition.Value}
			}
		}
	}
	for _, value := range values {
		if actual == requestInterceptionValueString(value) {
			return true
		}
	}
	return false
}

func requestInterceptionRegexMatches(pattern string, value string) bool {
	if strings.TrimSpace(pattern) == "" {
		return false
	}
	matched, err := regexp.MatchString(pattern, value)
	return err == nil && matched
}

func requestInterceptionNumberMatches(operator string, actual float64, expected any) bool {
	value, ok := requestInterceptionNumberValue(expected)
	if !ok {
		return false
	}
	switch operator {
	case "gt":
		return actual > value
	case "gte":
		return actual >= value
	case "lt":
		return actual < value
	case "lte":
		return actual <= value
	default:
		return false
	}
}

func requestInterceptionNumberValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func newRequestInterceptionError(rule operation_setting.RequestInterceptionRule) *types.NewAPIError {
	statusCode := rule.Error.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusBadRequest
	}
	errorType := strings.TrimSpace(rule.Error.Type)
	if errorType == "" {
		errorType = "invalid_request_error"
	}
	errorCode := strings.TrimSpace(rule.Error.Code)
	if errorCode == "" {
		errorCode = string(types.ErrorCodeInvalidRequest)
	}
	message := strings.TrimSpace(rule.Error.Message)
	if message == "" {
		message = fmt.Sprintf("request rejected by interception rule %q", rule.Name)
	}

	return types.WithOpenAIError(
		types.OpenAIError{
			Message: message,
			Type:    errorType,
			Code:    errorCode,
		},
		statusCode,
		types.ErrOptionWithSkipRetry(),
		types.ErrOptionWithNoRecordErrorLog(),
	)
}

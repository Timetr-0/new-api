package operation_setting

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

type RequestInterceptionCondition struct {
	Path     string `json:"path"`
	Operator string `json:"operator,omitempty"`
	Value    any    `json:"value,omitempty"`
	Values   []any  `json:"values,omitempty"`
}

type RequestInterceptionError struct {
	StatusCode int    `json:"status_code"`
	Type       string `json:"type,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
}

type RequestInterceptionRule struct {
	Name             string                         `json:"name"`
	Disabled         bool                           `json:"disabled,omitempty"`
	ChannelTypes     []int                          `json:"channel_types,omitempty"`
	ChannelIds       []int                          `json:"channel_ids,omitempty"`
	ChannelNameRegex []string                       `json:"channel_name_regex,omitempty"`
	RelayFormats     []string                       `json:"relay_formats,omitempty"`
	ModelRegex       []string                       `json:"model_regex,omitempty"`
	PathRegex        []string                       `json:"path_regex,omitempty"`
	Methods          []string                       `json:"methods,omitempty"`
	Conditions       []RequestInterceptionCondition `json:"conditions,omitempty"`
	AnyConditions    []RequestInterceptionCondition `json:"any_conditions,omitempty"`
	Error            RequestInterceptionError       `json:"error"`
}

type RequestInterceptionSetting struct {
	Enabled bool                      `json:"enabled"`
	Rules   []RequestInterceptionRule `json:"rules"`
}

const DefaultAwsThinkingForcedToolChoiceMessage = "ValidationException: Thinking may not be enabled when tool_choice forces tool use."

func DefaultRequestInterceptionRules() []RequestInterceptionRule {
	return []RequestInterceptionRule{
		{
			Name:         "aws claude thinking with forced tool choice",
			ChannelTypes: []int{constant.ChannelTypeAws},
			RelayFormats: []string{string(types.RelayFormatClaude)},
			ModelRegex:   []string{"^claude-.*"},
			PathRegex:    []string{"/v1/messages", "/v1/chat/completions", "/v1/responses"},
			Methods:      []string{http.MethodPost},
			Conditions: []RequestInterceptionCondition{
				{Path: "thinking.type", Operator: "exists"},
				{Path: "tool_choice.type", Operator: "in", Values: []any{"any", "tool"}},
			},
			Error: RequestInterceptionError{
				StatusCode: http.StatusBadRequest,
				Type:       "invalid_request_error",
				Code:       string(types.ErrorCodeInvalidRequest),
				Message:    DefaultAwsThinkingForcedToolChoiceMessage,
			},
		},
	}
}

var requestInterceptionSetting = RequestInterceptionSetting{
	Enabled: true,
	Rules:   DefaultRequestInterceptionRules(),
}

func init() {
	config.GlobalConfig.Register("request_interception_setting", &requestInterceptionSetting)
}

func GetRequestInterceptionSetting() *RequestInterceptionSetting {
	return &requestInterceptionSetting
}

func ValidateRequestInterceptionRulesJSON(value string) error {
	raw := strings.TrimSpace(value)
	if raw == "" {
		raw = "[]"
	}

	var jsonValue any
	if err := common.UnmarshalJsonStr(raw, &jsonValue); err != nil {
		return fmt.Errorf("invalid JSON array: %w", err)
	}
	if _, ok := jsonValue.([]any); !ok {
		return fmt.Errorf("rules must be a JSON array")
	}

	var rules []RequestInterceptionRule
	if err := common.UnmarshalJsonStr(raw, &rules); err != nil {
		return fmt.Errorf("invalid JSON array: %w", err)
	}
	return ValidateRequestInterceptionRules(rules)
}

func ValidateRequestInterceptionRules(rules []RequestInterceptionRule) error {
	for index, rule := range rules {
		if strings.TrimSpace(rule.Name) == "" {
			return fmt.Errorf("rule %d: name is required", index+1)
		}
		if err := validateRequestInterceptionRegexes(index, "channel_name_regex", rule.ChannelNameRegex); err != nil {
			return err
		}
		if err := validateRequestInterceptionRegexes(index, "model_regex", rule.ModelRegex); err != nil {
			return err
		}
		if err := validateRequestInterceptionRegexes(index, "path_regex", rule.PathRegex); err != nil {
			return err
		}
		if len(rule.Conditions) == 0 && len(rule.AnyConditions) == 0 {
			return fmt.Errorf("rule %d: at least one condition is required", index+1)
		}
		for conditionIndex, condition := range rule.Conditions {
			if err := validateRequestInterceptionCondition(index, conditionIndex, condition); err != nil {
				return err
			}
		}
		for conditionIndex, condition := range rule.AnyConditions {
			if err := validateRequestInterceptionCondition(index, conditionIndex, condition); err != nil {
				return err
			}
		}
		if rule.Error.StatusCode != 0 && (rule.Error.StatusCode < 100 || rule.Error.StatusCode > 599) {
			return fmt.Errorf("rule %d: error.status_code must be between 100 and 599", index+1)
		}
		if strings.TrimSpace(rule.Error.Message) == "" {
			return fmt.Errorf("rule %d: error.message is required", index+1)
		}
	}
	return nil
}

func validateRequestInterceptionRegexes(ruleIndex int, fieldName string, patterns []string) error {
	for patternIndex, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("rule %d: %s[%d] cannot be empty", ruleIndex+1, fieldName, patternIndex)
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("rule %d: invalid %s[%d]: %w", ruleIndex+1, fieldName, patternIndex, err)
		}
	}
	return nil
}

func validateRequestInterceptionCondition(ruleIndex int, conditionIndex int, condition RequestInterceptionCondition) error {
	if strings.TrimSpace(condition.Path) == "" {
		return fmt.Errorf("rule %d: condition %d path is required", ruleIndex+1, conditionIndex+1)
	}
	switch strings.ToLower(strings.TrimSpace(condition.Operator)) {
	case "", "exists", "not_exists", "eq", "equals", "ne", "not_equals", "in", "not_in", "regex", "not_regex", "contains", "not_contains", "gt", "gte", "lt", "lte":
		return nil
	default:
		return fmt.Errorf("rule %d: condition %d has unsupported operator %q", ruleIndex+1, conditionIndex+1, condition.Operator)
	}
}

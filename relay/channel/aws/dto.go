package aws

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
)

type AwsClaudeRequest struct {
	// AnthropicVersion should be "bedrock-2023-05-31"
	AnthropicVersion  string              `json:"anthropic_version"`
	AnthropicBeta     json.RawMessage     `json:"anthropic_beta,omitempty"`
	System            any                 `json:"system,omitempty"`
	Messages          []dto.ClaudeMessage `json:"messages"`
	MaxTokens         *uint               `json:"max_tokens,omitempty"`
	Temperature       *float64            `json:"temperature,omitempty"`
	TopP              *float64            `json:"top_p,omitempty"`
	TopK              *int                `json:"top_k,omitempty"`
	StopSequences     []string            `json:"stop_sequences,omitempty"`
	Tools             any                 `json:"tools,omitempty"`
	ToolChoice        any                 `json:"tool_choice,omitempty"`
	ContextManagement json.RawMessage     `json:"context_management,omitempty"`
	Thinking          *dto.Thinking       `json:"thinking,omitempty"`
	OutputConfig      json.RawMessage     `json:"output_config,omitempty"`
	//Metadata         json.RawMessage     `json:"metadata,omitempty"`
}

func formatRequest(requestBody io.Reader, requestHeader http.Header) (*AwsClaudeRequest, error) {
	var awsClaudeRequest AwsClaudeRequest
	err := common.DecodeJson(requestBody, &awsClaudeRequest)
	if err != nil {
		return nil, err
	}
	awsClaudeRequest.AnthropicVersion = "bedrock-2023-05-31"

	// check header anthropic-beta
	anthropicBetaValues := requestHeader.Get("anthropic-beta")
	if len(anthropicBetaValues) > 0 {
		var tempArray []string
		for _, v := range strings.Split(anthropicBetaValues, ",") {
			v = strings.TrimSpace(v)
			if v != "" {
				tempArray = append(tempArray, v)
			}
		}
		if len(tempArray) > 0 {
			betaJson, err := json.Marshal(tempArray)
			if err != nil {
				return nil, err
			}
			awsClaudeRequest.AnthropicBeta = betaJson
		}
	}
	logger.LogJson(context.Background(), "json", awsClaudeRequest)
	return &awsClaudeRequest, nil
}

// NovaMessage Nova妯″瀷浣跨敤messages-v1鏍煎紡
type NovaMessage struct {
	Role    string        `json:"role"`
	Content []NovaContent `json:"content"`
}

type NovaContent struct {
	Text string `json:"text"`
}

type NovaRequest struct {
	SchemaVersion   string               `json:"schemaVersion"`             // 璇锋眰鐗堟湰锛屼緥濡?"1.0"
	Messages        []NovaMessage        `json:"messages"`                  // 瀵硅瘽娑堟伅鍒楄〃
	InferenceConfig *NovaInferenceConfig `json:"inferenceConfig,omitempty"` // 鎺ㄧ悊閰嶇疆锛屽彲閫?}

type NovaInferenceConfig struct {
	MaxTokens     int      `json:"maxTokens,omitempty"`     // 鏈€澶х敓鎴愮殑 token 鏁?	Temperature   float64  `json:"temperature,omitempty"`   // 闅忔満鎬?(榛樿 0.7, 鑼冨洿 0-1)
	TopP          float64  `json:"topP,omitempty"`          // nucleus sampling (榛樿 0.9, 鑼冨洿 0-1)
	TopK          int      `json:"topK,omitempty"`          // 闄愬埗鍊欓€?token 鏁?(榛樿 50, 鑼冨洿 0-128)
	StopSequences []string `json:"stopSequences,omitempty"` // 鍋滄鐢熸垚鐨勫簭鍒?}

// 杞崲OpenAI璇锋眰涓篘ova鏍煎紡
func convertToNovaRequest(req *dto.GeneralOpenAIRequest) *NovaRequest {
	novaMessages := make([]NovaMessage, len(req.Messages))
	for i, msg := range req.Messages {
		novaMessages[i] = NovaMessage{
			Role:    msg.Role,
			Content: []NovaContent{{Text: msg.StringContent()}},
		}
	}

	novaReq := &NovaRequest{
		SchemaVersion: "messages-v1",
		Messages:      novaMessages,
	}

	// 璁剧疆鎺ㄧ悊閰嶇疆
	if (req.MaxTokens != nil && *req.MaxTokens != 0) || (req.Temperature != nil && *req.Temperature != 0) || (req.TopP != nil && *req.TopP != 0) || (req.TopK != nil && *req.TopK != 0) || req.Stop != nil {
		novaReq.InferenceConfig = &NovaInferenceConfig{}
		if req.MaxTokens != nil && *req.MaxTokens != 0 {
			novaReq.InferenceConfig.MaxTokens = int(*req.MaxTokens)
		}
		if req.Temperature != nil && *req.Temperature != 0 {
			novaReq.InferenceConfig.Temperature = *req.Temperature
		}
		if req.TopP != nil && *req.TopP != 0 {
			novaReq.InferenceConfig.TopP = *req.TopP
		}
		if req.TopK != nil && *req.TopK != 0 {
			novaReq.InferenceConfig.TopK = *req.TopK
		}
		if req.Stop != nil {
			if stopSequences := parseStopSequences(req.Stop); len(stopSequences) > 0 {
				novaReq.InferenceConfig.StopSequences = stopSequences
			}
		}
	}

	return novaReq
}

// parseStopSequences 瑙ｆ瀽鍋滄搴忓垪锛屾敮鎸佸瓧绗︿覆鎴栧瓧绗︿覆鏁扮粍
func parseStopSequences(stop any) []string {
	if stop == nil {
		return nil
	}

	switch v := stop.(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []string:
		return v
	case []interface{}:
		var sequences []string
		for _, item := range v {
			if str, ok := item.(string); ok && str != "" {
				sequences = append(sequences, str)
			}
		}
		return sequences
	}
	return nil
}

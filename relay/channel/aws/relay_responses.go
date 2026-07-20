package aws

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/types"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrockruntimeTypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func awsResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, awsResp *bedrockruntime.InvokeModelOutput) (*types.NewAPIError, *dto.Usage) {
	if awsResp == nil {
		return types.NewOpenAIError(fmt.Errorf("invalid AWS response"), types.ErrorCodeBadResponse, http.StatusInternalServerError), nil
	}
	if awsResp.ContentType != nil && *awsResp.ContentType != "" {
		c.Writer.Header().Set("Content-Type", *awsResp.ContentType)
	}
	return awsWriteResponsesBody(c, info, awsResp.Body, nil)
}

func awsResponsesHTTPHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	apiErr, usage := awsWriteResponsesBody(c, info, body, resp)
	return usage, apiErr
}

func awsWriteResponsesBody(c *gin.Context, info *relaycommon.RelayInfo, body []byte, src *http.Response) (*types.NewAPIError, *dto.Usage) {
	var claudeResponse dto.ClaudeResponse
	if err := common.Unmarshal(body, &claudeResponse); err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError), nil
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		statusCode := http.StatusInternalServerError
		if src != nil {
			statusCode = src.StatusCode
		}
		return types.WithClaudeError(*claudeError, statusCode), nil
	}
	awsMarkClaudeResponseMetadata(c, &claudeResponse)
	if c != nil {
		claudeResponse.Id = helper.GetResponseID(c)
	}

	convertResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAIResponses, &claudeResponse)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError), nil
	}
	responsesResp, ok := convertResult.Value.(*dto.OpenAIResponsesResponse)
	if !ok {
		return types.NewOpenAIError(fmt.Errorf("expected OpenAI responses response, got %T", convertResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError), nil
	}
	if responsesResp.Model == "" {
		responsesResp.Model = info.UpstreamModelName
	}

	usage := convertResult.Usage
	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		responsesResp.Usage = relayconvert.UsageFromChatUsage(usage)
	} else if responsesResp.Usage == nil {
		responsesResp.Usage = relayconvert.UsageFromChatUsage(usage)
	}

	responseBody, err := common.Marshal(responsesResp)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError), nil
	}
	if c != nil && c.Writer != nil && c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	service.IOCopyBytesGracefully(c, src, responseBody)
	return nil, usage
}

func awsResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, awsResp *bedrockruntime.InvokeModelWithResponseStreamOutput) (*types.NewAPIError, *dto.Usage) {
	if awsResp == nil {
		return types.NewOpenAIError(fmt.Errorf("invalid AWS stream response"), types.ErrorCodeBadResponse, http.StatusInternalServerError), nil
	}
	stream := awsResp.GetStream()
	defer stream.Close()

	converter, apiErr := newAWSResponsesStreamConverter(c, info)
	if apiErr != nil {
		return apiErr, nil
	}

	for event := range stream.Events() {
		switch v := event.(type) {
		case *bedrockruntimeTypes.ResponseStreamMemberChunk:
			info.SetFirstResponseTime()
			if apiErr := converter.HandleClaudeChunk(v.Value.Bytes); apiErr != nil {
				return apiErr, nil
			}
		case *bedrockruntimeTypes.UnknownUnionMember:
			fmt.Println("unknown tag:", v.Tag)
			return types.NewError(errors.New("unknown response type"), types.ErrorCodeInvalidRequest), nil
		default:
			fmt.Println("union is nil or unknown type")
			return types.NewError(errors.New("nil or unknown response type"), types.ErrorCodeInvalidRequest), nil
		}
	}

	usage, apiErr := converter.Finalize()
	return apiErr, usage
}

func awsResponsesHTTPStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	converter, apiErr := newAWSResponsesStreamConverter(c, info)
	if apiErr != nil {
		return nil, apiErr
	}

	var streamErr *types.NewAPIError
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		streamErr = converter.HandleClaudeChunk([]byte(data))
		if streamErr != nil {
			sr.Stop(streamErr)
		}
	})
	if streamErr != nil {
		return nil, streamErr
	}
	return converter.Finalize()
}

type awsResponsesStreamConverter struct {
	c          *gin.Context
	info       *relaycommon.RelayInfo
	state      *relayconvert.ResponseStreamState
	claudeInfo *claude.ClaudeResponseInfo
}

func newAWSResponsesStreamConverter(c *gin.Context, info *relaycommon.RelayInfo) (*awsResponsesStreamConverter, *types.NewAPIError) {
	responseID := helper.GetResponseID(c)
	created := common.GetTimestamp()
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatClaude, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{
		ID:      responseID,
		Model:   info.UpstreamModelName,
		Created: created,
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	helper.SetEventStreamHeaders(c)
	return &awsResponsesStreamConverter{
		c:     c,
		info:  info,
		state: state,
		claudeInfo: &claude.ClaudeResponseInfo{
			ResponseId:   responseID,
			Created:      created,
			Model:        info.UpstreamModelName,
			ResponseText: strings.Builder{},
			Usage:        &dto.Usage{},
		},
	}, nil
}

func (s *awsResponsesStreamConverter) HandleClaudeChunk(data []byte) *types.NewAPIError {
	var claudeResponse dto.ClaudeResponse
	if err := common.Unmarshal(data, &claudeResponse); err != nil {
		common.SysLog("error unmarshalling AWS Claude stream response: " + err.Error())
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		return types.WithClaudeError(*claudeError, http.StatusInternalServerError)
	}
	awsMarkClaudeResponseMetadata(s.c, &claudeResponse)
	if claudeResponse.Type == "message_start" && claudeResponse.Message != nil && claudeResponse.Message.Model != "" {
		s.info.UpstreamModelName = claudeResponse.Message.Model
	}
	claude.FormatClaudeResponseInfo(&claudeResponse, nil, s.claudeInfo)
	if claudeResponse.Type == "message_delta" {
		claudeResponse.Usage = relayconvert.BuildMessageDeltaPatchUsage(&claudeResponse, s.claudeInfo)
	}

	results, err := relayconvert.ConvertStreamResponseChunk(s.c, s.info, s.state, &claudeResponse)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	for _, result := range results {
		event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
		if !ok {
			return types.NewOpenAIError(fmt.Errorf("expected OAI responses stream event, got %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		if err := awsSendResponsesStreamEvent(s.c, event); err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	return nil
}

func (s *awsResponsesStreamConverter) Finalize() (*dto.Usage, *types.NewAPIError) {
	usage := s.finalUsage()
	if usage != nil {
		s.state.SetUsage(usage)
	}

	finalResults, err := relayconvert.FinalizeStreamResponse(s.c, s.info, s.state)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	for _, result := range finalResults {
		event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
		if !ok {
			return nil, types.NewOpenAIError(fmt.Errorf("expected OAI responses stream event, got %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		if err := awsSendResponsesStreamEvent(s.c, event); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	return usage, nil
}

func (s *awsResponsesStreamConverter) finalUsage() *dto.Usage {
	if s.claudeInfo != nil && s.claudeInfo.Usage != nil {
		usage := s.claudeInfo.Usage
		if usage.PromptTokens == 0 || usage.CompletionTokens == 0 || !s.claudeInfo.Done {
			fallback := service.ResponseText2Usage(s.c, s.claudeInfo.ResponseText.String(), s.info.UpstreamModelName, s.info.GetEstimatePromptTokens())
			if usage.CompletionTokens == 0 || (!s.claudeInfo.Done && fallback.CompletionTokens > usage.CompletionTokens) {
				usage.CompletionTokens = fallback.CompletionTokens
			}
			if usage.PromptTokens == 0 {
				usage.PromptTokens = fallback.PromptTokens
			}
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		usage.UsageSemantic = "anthropic"
		if usage.BillingUsage == nil {
			usage.BillingUsage = dto.NewClaudeMessagesBillingUsage(relayconvert.BuildMessageDeltaPatchUsage(nil, s.claudeInfo))
		}
		if mapped := relayconvert.UsageFromClaudeUsage(usage); mapped != nil && mapped.TotalTokens > 0 {
			return mapped
		}
	}

	usage := s.state.Usage()
	if usage != nil && usage.TotalTokens > 0 {
		return usage
	}
	text := s.state.UsageText()
	if text == "" && s.claudeInfo != nil {
		text = s.claudeInfo.ResponseText.String()
	}
	return service.ResponseText2Usage(s.c, text, s.info.UpstreamModelName, s.info.GetEstimatePromptTokens())
}

func awsSendResponsesStreamEvent(c *gin.Context, event relayconvert.ChatToResponsesStreamEvent) error {
	data, err := common.Marshal(event.Payload)
	if err != nil {
		return err
	}
	return helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: event.Type}, string(data))
}

func awsMarkClaudeResponseMetadata(c *gin.Context, claudeResponse *dto.ClaudeResponse) {
	if c == nil || claudeResponse == nil {
		return
	}
	if strings.EqualFold(claudeResponse.StopReason, "refusal") {
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "claude_stop_reason=refusal")
	}
	if claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil && strings.EqualFold(*claudeResponse.Delta.StopReason, "refusal") {
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "claude_stop_reason=refusal")
	}
	if claudeResponse.Usage != nil && claudeResponse.Usage.ServerToolUse != nil && claudeResponse.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", claudeResponse.Usage.ServerToolUse.WebSearchRequests)
	}
}

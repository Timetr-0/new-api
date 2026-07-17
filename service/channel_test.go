package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestShouldDisableChannelForBedrockDailyTokenThrottle(t *testing.T) {
	oldEnabled := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldEnabled
	})

	err := &types.NewAPIError{
		Err: errors.New("InvokeModel: operation error Bedrock Runtime: InvokeModel, exceeded maximum number of attempts, 3, https response error StatusCode: 429, RequestID: test, ThrottlingException: Too many tokens per day, please wait before trying again."),
		StatusCode: 429,
	}

	require.True(t, ShouldDisableChannel(err))
}

func TestShouldDisableChannelDoesNotDisableGenericBedrockThrottle(t *testing.T) {
	oldEnabled := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldEnabled
	})

	err := &types.NewAPIError{
		Err:        errors.New("InvokeModel: operation error Bedrock Runtime: InvokeModel, https response error StatusCode: 429, ThrottlingException: Rate exceeded"),
		StatusCode: 429,
	}

	require.False(t, ShouldDisableChannel(err))
}

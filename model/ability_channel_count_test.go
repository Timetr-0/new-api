package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetAbilityChannelCountTestTables(t *testing.T, memoryCacheEnabled bool) {
	t.Helper()
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = memoryCacheEnabled
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}))
	for _, table := range []string{"abilities", "channels"} {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	InitChannelCache()
	t.Cleanup(func() {
		for _, table := range []string{"abilities", "channels"} {
			require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
		}
		InitChannelCache()
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})
}

func insertAbilityChannelCountCandidate(
	t *testing.T,
	channelID int,
	group string,
	modelName string,
	channelType int,
	channelStatus int,
	abilityEnabled bool,
) {
	t.Helper()
	require.NoError(t, DB.Create(&Channel{
		Id:     channelID,
		Type:   channelType,
		Key:    fmt.Sprintf("key-%d", channelID),
		Status: channelStatus,
		Name:   fmt.Sprintf("channel-%d", channelID),
		Models: modelName,
		Group:  group,
	}).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: channelID,
		Enabled:   abilityEnabled,
	}).Error)
}

func TestCountEnabledChannelsByGroupModelAndTypeDB(t *testing.T) {
	resetAbilityChannelCountTestTables(t, false)

	insertAbilityChannelCountCandidate(t, 1, "default", "claude-test", constant.ChannelTypeAws, common.ChannelStatusEnabled, true)
	insertAbilityChannelCountCandidate(t, 2, "default", "claude-test", constant.ChannelTypeAws, common.ChannelStatusEnabled, true)
	insertAbilityChannelCountCandidate(t, 3, "default", "claude-test", constant.ChannelTypeAws, common.ChannelStatusManuallyDisabled, true)
	insertAbilityChannelCountCandidate(t, 4, "default", "claude-test", constant.ChannelTypeOpenAI, common.ChannelStatusEnabled, true)
	insertAbilityChannelCountCandidate(t, 5, "vip", "claude-test", constant.ChannelTypeAws, common.ChannelStatusEnabled, true)
	insertAbilityChannelCountCandidate(t, 6, "default", "claude-test", constant.ChannelTypeAws, common.ChannelStatusEnabled, false)

	count, err := CountEnabledChannelsByGroupModelAndType("default", "claude-test", constant.ChannelTypeAws)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestCountEnabledChannelsByGroupModelAndTypeCache(t *testing.T) {
	resetAbilityChannelCountTestTables(t, true)

	insertAbilityChannelCountCandidate(t, 1, "default", "claude-test", constant.ChannelTypeAws, common.ChannelStatusEnabled, true)
	insertAbilityChannelCountCandidate(t, 2, "default", "claude-test", constant.ChannelTypeAws, common.ChannelStatusEnabled, true)
	insertAbilityChannelCountCandidate(t, 3, "default", "claude-test", constant.ChannelTypeOpenAI, common.ChannelStatusEnabled, true)
	InitChannelCache()

	count, err := CountEnabledChannelsByGroupModelAndType("default", "claude-test", constant.ChannelTypeAws)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

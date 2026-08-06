package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserAutoGroupUsesConfiguredAutoGroupsWithoutUsableGroupIntersection(t *testing.T) {
	originAutoGroups := setting.AutoGroups2JsonString()
	originUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originUsableGroups))
	})

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["accountC","accountD","accountE"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"auto":"auto","default":"default"}`))

	got := GetUserAutoGroup("default")

	assert.Equal(t, []string{"accountC", "accountD", "accountE"}, got)
}

func TestGetUserAutoGroupByNameUsesConfiguredAutoGroupName(t *testing.T) {
	originAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originAutoGroups))
	})

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`{
		"auto": ["accountA"],
		"auto_eu": ["accountC", "auto", "accountD"]
	}`))

	assert.Equal(t, []string{"accountA"}, GetUserAutoGroup("default"))
	assert.Equal(t, []string{"accountC", "accountD"}, GetUserAutoGroupByName("default", "auto_eu"))
	assert.Empty(t, GetUserAutoGroupByName("default", "auto_missing"))
}

package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateAutoGroupsByJsonStringSupportsLegacyList(t *testing.T) {
	originAutoGroups := AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, UpdateAutoGroupsByJsonString(originAutoGroups))
	})

	require.NoError(t, UpdateAutoGroupsByJsonString(`["default","auto_eu","vip","vip",""]`))

	assert.Equal(t, []string{"default", "vip"}, GetAutoGroups())
	assert.Equal(t, []string{"auto"}, GetAutoGroupNames())
	assert.True(t, ContainsAutoGroup("auto"))
	assert.False(t, ContainsAutoGroup("auto_eu"))
}

func TestUpdateAutoGroupsByJsonStringSupportsMultipleAutoGroups(t *testing.T) {
	originAutoGroups := AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, UpdateAutoGroupsByJsonString(originAutoGroups))
	})

	require.NoError(t, UpdateAutoGroupsByJsonString(`{
		"auto_eu": ["eu-a", "auto", "eu-b", "auto_west", "eu-a"],
		"auto_us": ["us-a"]
	}`))

	assert.Equal(t, []string{"eu-a", "eu-b"}, GetAutoGroupsForGroup("auto_eu"))
	assert.Equal(t, []string{"us-a"}, GetAutoGroupsForGroup("auto_us"))
	assert.Equal(t, []string{"auto_eu", "auto_us"}, GetAutoGroupNames())
	assert.True(t, ContainsAutoGroup("auto_eu"))
	assert.False(t, ContainsAutoGroup("auto"))
}

func TestUpdateAutoGroupsByJsonStringIgnoresNonAutoGroupKeys(t *testing.T) {
	originAutoGroups := AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, UpdateAutoGroupsByJsonString(originAutoGroups))
	})

	require.NoError(t, UpdateAutoGroupsByJsonString(`{
		"auto":["default"],
		"default":["vip"],
		"vip":["default"]
	}`))

	assert.Equal(t, []string{"default"}, GetAutoGroups())
	assert.Equal(t, []string{"auto"}, GetAutoGroupNames())
}

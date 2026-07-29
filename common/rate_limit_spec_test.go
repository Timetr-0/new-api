package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRateLimitSpec(t *testing.T) {
	fixed, err := ParseRateLimitSpec("700", false)
	require.NoError(t, err)
	assert.False(t, fixed.Dynamic)
	assert.Equal(t, 700, fixed.Fixed)

	dynamic, err := ParseRateLimitSpec("N(700,std=100)", false)
	require.NoError(t, err)
	assert.True(t, dynamic.Dynamic)
	assert.Equal(t, 700.0, dynamic.Mean)
	assert.Equal(t, 100.0, dynamic.StdDev)

	_, err = ParseRateLimitSpec("0", false)
	require.Error(t, err)

	optional, err := ParseRateLimitSpec("0", true)
	require.NoError(t, err)
	assert.Equal(t, 0, optional.Fixed)
}

func TestResolveRateLimitSpecCachesDynamicValuePerMinute(t *testing.T) {
	first, err := ResolveRateLimitSpec("test-dynamic", "N(700,std=100)", false)
	require.NoError(t, err)
	second, err := ResolveRateLimitSpec("test-dynamic", "N(700,std=100)", false)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.GreaterOrEqual(t, first, 1)
}

package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPricingReflectsSavedOptionChangesImmediately(t *testing.T) {
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() { common.OptionMap = previousOptions })

	for key, previous := range map[string]string{
		"ModelRatio":      ratio_setting.ModelRatio2JSONString(),
		"CompletionRatio": ratio_setting.CompletionRatio2JSONString(),
		"ModelPrice":      ratio_setting.ModelPrice2JSONString(),
		"CacheRatio":      ratio_setting.CacheRatio2JSONString(),
	} {
		t.Cleanup(func() { require.NoError(t, updateOptionMap(key, previous)) })
	}

	for _, test := range []struct {
		name  string
		key   string
		value string
	}{
		{"input price", "ModelRatio", `{"pricing-update-model":1.5}`},
		{"output price", "CompletionRatio", `{"pricing-update-model":2}`},
		{"per-request price", "ModelPrice", `{"pricing-update-model":0.25}`},
		{"cache price", "CacheRatio", `{"pricing-update-model":0.2}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetPricingEndpointTestTables(t)
			insertPricingEndpointChannel(t, 401, constant.ChannelTypeOpenAI, dto.ChannelOtherSettings{})
			insertPricingEndpointAbility(t, 401, "pricing-update-model")
			require.NoError(t, updateOptionMap("ModelRatio", `{"pricing-update-model":37.5}`))
			require.NoError(t, updateOptionMap("CompletionRatio", `{"pricing-update-model":1}`))
			require.NoError(t, updateOptionMap("ModelPrice", `{}`))
			require.NoError(t, updateOptionMap("CacheRatio", `{"pricing-update-model":0.1}`))
			InitChannelCache()
			initial := GetPricing()
			require.Len(t, initial, 1)
			require.Equal(t, float64(75), initial[0].ModelRatio*2)

			require.NoError(t, updateOptionMap(test.key, test.value))
			updated := GetPricing()
			require.Len(t, updated, 1)
			switch test.key {
			case "ModelRatio":
				assert.Equal(t, float64(3), updated[0].ModelRatio*2)
			case "CompletionRatio":
				assert.Equal(t, float64(150), updated[0].ModelRatio*2*updated[0].CompletionRatio)
			case "ModelPrice":
				assert.Equal(t, 1, updated[0].QuotaType)
				assert.Equal(t, 0.25, updated[0].ModelPrice)
			case "CacheRatio":
				require.NotNil(t, updated[0].CacheRatio)
				assert.Equal(t, 0.2, *updated[0].CacheRatio)
			}
		})
	}
}

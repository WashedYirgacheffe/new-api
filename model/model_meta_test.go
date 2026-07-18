package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelSourceMetadataPersistsWithoutChangingChannelProviders(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Model{}, &ModelChannelProvider{}))

	var created Model
	require.NoError(t, common.Unmarshal([]byte(`{
		"model_name":"test/source-metadata-persistence",
		"display_name":"Source Metadata Persistence",
		"source_metadata":"{\"provider\":\"dmxapi\",\"upstream_id\":\"gpt-4o\",\"revision\":1}",
		"channel_providers":[" DMXAPI ","deepwl","dmxapi"]
	}`), &created))
	require.NoError(t, created.Insert())
	t.Cleanup(func() {
		DB.Where("model_id = ?", created.Id).Delete(&ModelChannelProvider{})
		DB.Unscoped().Delete(&Model{}, created.Id)
	})

	var stored Model
	require.NoError(t, DB.First(&stored, created.Id).Error)
	assert.Equal(t, `{"provider":"dmxapi","upstream_id":"gpt-4o","revision":1}`, stored.SourceMetadata)

	providers, err := GetChannelProvidersByModelsMap([]int{created.Id})
	require.NoError(t, err)
	assert.Equal(t, []string{"deepwl", "dmxapi"}, providers[created.Id])

	stored.SourceMetadata = `{"provider":"dmxapi","upstream_id":"gpt-4.1","revision":2,"limits":{"rpm":120}}`
	stored.ChannelProviders = nil
	require.NoError(t, stored.Update())

	var updated Model
	require.NoError(t, DB.First(&updated, created.Id).Error)
	assert.Equal(t, stored.SourceMetadata, updated.SourceMetadata)

	providers, err = GetChannelProvidersByModelsMap([]int{created.Id})
	require.NoError(t, err)
	assert.Equal(t, []string{"deepwl", "dmxapi"}, providers[created.Id])

	response, err := common.Marshal(&updated)
	require.NoError(t, err)
	var responseBody map[string]interface{}
	require.NoError(t, common.Unmarshal(response, &responseBody))
	assert.Equal(t, updated.SourceMetadata, responseBody["source_metadata"])
}

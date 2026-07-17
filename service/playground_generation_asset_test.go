package service

import (
	"encoding/base64"
	"errors"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPlaygroundGenerationAssetTest(t *testing.T) *model.PlaygroundGeneration {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(
		&model.User{},
		&model.PlaygroundGeneration{},
		&model.PlaygroundGenerationAsset{},
	))
	require.NoError(t, model.DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.PlaygroundGenerationAsset{}).Error)
	require.NoError(t, model.DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.PlaygroundGeneration{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", 101).Delete(&model.User{}).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id:       101,
		Username: "pg-asset-user",
		Password: "playground-test-password",
		Role:     1,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "pg-asset-00000000000000000000101",
	}).Error)
	t.Cleanup(func() {
		model.DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.PlaygroundGenerationAsset{})
		model.DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.PlaygroundGeneration{})
		model.DB.Unscoped().Where("id = ?", 101).Delete(&model.User{})
	})
	t.Setenv(PlaygroundAssetDirectoryEnv, t.TempDir())
	generation, err := model.CreatePlaygroundGeneration(101, model.PlaygroundGenerationCreate{
		Operation:       model.PlaygroundGenerationOperationImage,
		Model:           "deepwl/test-inline-image",
		Group:           "default",
		Prompt:          "one pixel",
		Parameters:      map[string]interface{}{},
		ContractHash:    "contract-hash",
		ContractVersion: 1,
		PricingVersion:  "pricing-version",
	})
	require.NoError(t, err)
	return generation
}

func TestPersistResolveAndDeletePlaygroundGenerationAsset(t *testing.T) {
	generation := setupPlaygroundGenerationAssetTest(t)
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zr1sAAAAASUVORK5CYII=")
	require.NoError(t, err)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	result, err := PersistPlaygroundGenerationAsset(101, generation.Id, 0, dataURL)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Ordinal)
	assert.Equal(t, "image/png", result.MimeType)
	assert.EqualValues(t, len(png), result.SizeBytes)
	assert.Contains(t, result.URL, generation.Id)

	repeated, err := PersistPlaygroundGenerationAsset(101, generation.Id, 0, dataURL)
	require.NoError(t, err)
	assert.Equal(t, result, repeated)

	asset, path, err := ResolvePlaygroundGenerationAsset(101, generation.Id, 0)
	require.NoError(t, err)
	assert.Equal(t, "image/png", asset.MimeType)
	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, png, stored)

	_, _, err = ResolvePlaygroundGenerationAsset(202, generation.Id, 0)
	assert.ErrorIs(t, err, model.ErrPlaygroundGenerationAssetNotFound)

	require.NoError(t, DeletePlaygroundGenerationHistory(101, generation.Id))
	_, err = os.Stat(path)
	assert.True(t, errors.Is(err, os.ErrNotExist))
	_, err = model.GetPlaygroundGenerationById(101, generation.Id)
	assert.ErrorIs(t, err, model.ErrPlaygroundGenerationNotFound)
}

func TestPersistPlaygroundGenerationAssetRejectsMismatchedContent(t *testing.T) {
	generation := setupPlaygroundGenerationAssetTest(t)
	_, err := PersistPlaygroundGenerationAsset(
		101,
		generation.Id,
		0,
		"data:image/png;base64,"+base64.StdEncoding.EncodeToString([]byte("not a png")),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

func TestPersistPlaygroundGenerationAssetRejectsCorruptedExistingFile(t *testing.T) {
	generation := setupPlaygroundGenerationAssetTest(t)
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zr1sAAAAASUVORK5CYII=")
	require.NoError(t, err)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	require.NotNil(t, mustPersistPlaygroundGenerationAsset(t, generation.Id, dataURL))

	_, path, err := ResolvePlaygroundGenerationAsset(101, generation.Id, 0)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("corrupted"), 0o640))

	_, err = PersistPlaygroundGenerationAsset(101, generation.Id, 0, dataURL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match uploaded content")
}

func TestDeletePlaygroundGenerationKeepsHistoryWhenStorageIsUnavailable(t *testing.T) {
	generation := setupPlaygroundGenerationAssetTest(t)
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zr1sAAAAASUVORK5CYII=")
	require.NoError(t, err)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	require.NotNil(t, mustPersistPlaygroundGenerationAsset(t, generation.Id, dataURL))

	t.Setenv(PlaygroundAssetDirectoryEnv, "")
	err = DeletePlaygroundGenerationHistory(101, generation.Id)
	require.ErrorIs(t, err, ErrPlaygroundAssetStorageUnavailable)

	stored, err := model.GetPlaygroundGenerationById(101, generation.Id)
	require.NoError(t, err)
	assert.Equal(t, generation.Id, stored.Id)
}

func mustPersistPlaygroundGenerationAsset(t *testing.T, generationId string, dataURL string) *PlaygroundGenerationAssetResult {
	t.Helper()
	result, err := PersistPlaygroundGenerationAsset(101, generationId, 0, dataURL)
	require.NoError(t, err)
	return result
}

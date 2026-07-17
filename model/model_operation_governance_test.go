package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	governanceTestModelName  = "deepwl/governance-test-image"
	governanceTestProfileKey = "image.generate.governance-test"
)

func setupModelOperationGovernanceFixture(t *testing.T) (*ModelOperationProfile, *ModelOperationProfileVersion, *ModelOperationBinding) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(
		&Model{},
		&ModelOperationProfile{},
		&ModelOperationProfileVersion{},
		&ModelOperationBinding{},
		&ModelOperationBindingRevision{},
		&ModelOperationParameterEvidence{},
	))

	cleanup := func() {
		DB.Where("model_name = ?", governanceTestModelName).Delete(&ModelOperationParameterEvidence{})
		DB.Where("model_name = ?", governanceTestModelName).Delete(&ModelOperationBindingRevision{})
		DB.Where("model_name = ?", governanceTestModelName).Delete(&ModelOperationBinding{})
		var profile ModelOperationProfile
		if err := DB.Where("profile_key = ?", governanceTestProfileKey).First(&profile).Error; err == nil {
			DB.Where("profile_id = ?", profile.Id).Delete(&ModelOperationProfileVersion{})
			DB.Delete(&profile)
		}
		DB.Where("model_name = ?", governanceTestModelName).Delete(&Model{})
	}
	cleanup()
	t.Cleanup(cleanup)

	require.NoError(t, DB.Create(&Model{
		ModelName:   governanceTestModelName,
		DisplayName: "Governance test image",
		ModelType:   "image",
		Status:      1,
	}).Error)
	profile := &ModelOperationProfile{
		ProfileKey:  governanceTestProfileKey,
		DisplayName: "Governance test image",
	}
	version := &ModelOperationProfileVersion{
		Version:          1,
		Operation:        "image.generate",
		EndpointType:     "image-generation",
		ExecutionMode:    "sync",
		InputSchema:      `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"resolution":{"type":"string","enum":["1K","2K","4K"],"default":"1K"}},"required":["prompt"],"additionalProperties":false}`,
		UISchema:         `{"placements":{"prompt":"prompt","resolution":"footer"},"widgets":{"prompt":"textarea","resolution":"select"}}`,
		MaterialSchema:   `{}`,
		ResponseContract: "openai-image-generation-v1",
		SmokeTest:        `{"prompt":"test"}`,
		Status:           ModelOperationProfileStatusPublished,
	}
	require.NoError(t, SaveModelOperationProfileVersion(profile, version))
	binding := &ModelOperationBinding{
		ModelName:      governanceTestModelName,
		Operation:      version.Operation,
		ProfileKey:     profile.ProfileKey,
		ProfileVersion: version.Version,
		Overrides:      `{}`,
		Enabled:        true,
	}
	require.NoError(t, SaveModelOperationBindingWithExpectedHash(binding, ""))
	return profile, version, binding
}

func TestModelOperationBindingOptimisticLockRevisionAndRollback(t *testing.T) {
	profile, version, initial := setupModelOperationGovernanceFixture(t)
	initialHash := initial.ContractHash
	require.NotEmpty(t, initialHash)

	noChange := &ModelOperationBinding{
		ModelName: governanceTestModelName, Operation: version.Operation,
		ProfileKey: profile.ProfileKey, ProfileVersion: version.Version,
		Overrides: `{}`, Enabled: true,
	}
	require.NoError(t, SaveModelOperationBindingWithExpectedHash(noChange, initialHash))
	_, total, err := GetModelOperationBindingRevisions(governanceTestModelName, version.Operation, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)

	contractChange := &ModelOperationBinding{
		ModelName: governanceTestModelName, Operation: version.Operation,
		ProfileKey: profile.ProfileKey, ProfileVersion: version.Version,
		Overrides: `{"parameter_defaults":{"resolution":"2K"}}`, Enabled: true,
	}
	require.NoError(t, SaveModelOperationBindingWithExpectedHash(contractChange, initialHash))
	require.NotEqual(t, initialHash, contractChange.ContractHash)
	assert.Equal(t, 2, contractChange.ContractVersion)

	staleUpdate := &ModelOperationBinding{
		ModelName: governanceTestModelName, Operation: version.Operation,
		ProfileKey: profile.ProfileKey, ProfileVersion: version.Version,
		Overrides: contractChange.Overrides, Enabled: false,
	}
	err = SaveModelOperationBindingWithExpectedHash(staleUpdate, initialHash)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModelOperationBindingConflict)
	assert.Contains(t, err.Error(), "current hash")

	require.NoError(t, SaveModelOperationBindingWithExpectedHash(staleUpdate, contractChange.ContractHash))
	assert.False(t, staleUpdate.Enabled)
	assert.Equal(t, contractChange.ContractHash, staleUpdate.ContractHash)

	rolledBack, err := RollbackModelOperationBinding(governanceTestModelName, version.Operation, 1, staleUpdate.ContractHash)
	require.NoError(t, err)
	assert.True(t, rolledBack.Enabled)
	assert.Equal(t, initialHash, rolledBack.ContractHash)
	assert.JSONEq(t, `{}`, rolledBack.Overrides)

	revisions, total, err := GetModelOperationBindingRevisions(governanceTestModelName, version.Operation, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 4, total)
	require.Len(t, revisions, 4)
	assert.Equal(t, 4, revisions[0].Revision)
	assert.Equal(t, initialHash, revisions[0].ContractHash)
	assert.True(t, revisions[0].Enabled)
	assert.Equal(t, 3, revisions[1].Revision)
	assert.False(t, revisions[1].Enabled)
}

func TestModelOperationParameterEvidenceCRUD(t *testing.T) {
	setupModelOperationGovernanceFixture(t)
	evidence := &ModelOperationParameterEvidence{
		ModelName:          governanceTestModelName,
		Operation:          "image.generate",
		Field:              "resolution",
		SourceType:         ModelOperationEvidenceSourceDoc,
		SourceURL:          "https://doc.deepwl.cn/zh/models/image",
		SourceLocator:      "resolution",
		VerificationStatus: ModelOperationEvidenceStatusDocumented,
		VerifiedAt:         123456,
		Notes:              "Documented enum values",
	}
	require.NoError(t, SaveModelOperationParameterEvidence(evidence))
	require.Positive(t, evidence.Id)

	items, total, err := GetModelOperationParameterEvidence(governanceTestModelName, "image.generate", "resolution", 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, evidence.SourceURL, items[0].SourceURL)

	evidence.SourceType = ModelOperationEvidenceSourceTest
	evidence.VerificationStatus = ModelOperationEvidenceStatusTested
	evidence.Notes = "Verified against upstream"
	require.NoError(t, SaveModelOperationParameterEvidence(evidence))
	items, total, err = GetModelOperationParameterEvidence(governanceTestModelName, "", "", 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	assert.Equal(t, ModelOperationEvidenceStatusTested, items[0].VerificationStatus)
	assert.Equal(t, "Verified against upstream", items[0].Notes)

	require.NoError(t, DeleteModelOperationParameterEvidence(evidence.Id))
	_, total, err = GetModelOperationParameterEvidence(governanceTestModelName, "", "", 0, 20)
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.ErrorIs(t, DeleteModelOperationParameterEvidence(evidence.Id), gorm.ErrRecordNotFound)
}

func TestModelOperationParameterEvidenceOwnershipIsImmutable(t *testing.T) {
	setupModelOperationGovernanceFixture(t)
	evidence := &ModelOperationParameterEvidence{
		ModelName:          governanceTestModelName,
		Operation:          "image.generate",
		Field:              "resolution",
		SourceType:         ModelOperationEvidenceSourceManual,
		VerificationStatus: ModelOperationEvidenceStatusDocumented,
	}
	require.NoError(t, SaveModelOperationParameterEvidence(evidence))
	evidence.ModelName = "deepwl/another-model"
	err := SaveModelOperationParameterEvidence(evidence)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModelOperationParameterEvidenceConflict)
	assert.Contains(t, err.Error(), "cannot change")

	evidence.ModelName = governanceTestModelName
	evidence.Operation = "video.generate"
	err = SaveModelOperationParameterEvidence(evidence)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModelOperationParameterEvidenceConflict)
}

func TestDeleteModelOperationBindingUsesExpectedContractHash(t *testing.T) {
	_, version, binding := setupModelOperationGovernanceFixture(t)
	staleErr := DeleteModelOperationBinding(governanceTestModelName, version.Operation, "stale-hash")
	require.Error(t, staleErr)
	assert.ErrorIs(t, staleErr, ErrModelOperationBindingConflict)

	require.NoError(t, DeleteModelOperationBinding(governanceTestModelName, version.Operation, binding.ContractHash))
	var deleted ModelOperationBinding
	assert.ErrorIs(t, DB.Where("model_name = ? AND operation = ?", governanceTestModelName, version.Operation).First(&deleted).Error, gorm.ErrRecordNotFound)

	missingHashErr := DeleteModelOperationBinding(governanceTestModelName, version.Operation, "")
	require.Error(t, missingHashErr)
	assert.ErrorIs(t, missingHashErr, ErrModelOperationBindingConflict)
}

func TestSeedDefaultModelOperationParameterEvidenceIsIdempotent(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ModelOperationParameterEvidence{}))
	models := []string{
		"deepwl/gpt-image-2",
		"deepwl/gpt-image-2-c",
		"deepwl/gpt-image-2-all",
		"deepwl/omni-fast",
		"deepwl/omni-fast-v2v",
	}
	for _, modelName := range models {
		DB.Where("model_name = ?", modelName).Delete(&ModelOperationParameterEvidence{})
	}
	defer func() {
		for _, modelName := range models {
			DB.Where("model_name = ?", modelName).Delete(&ModelOperationParameterEvidence{})
		}
	}()

	require.NoError(t, SeedDefaultModelOperationParameterEvidence())
	var priceEvidence ModelOperationParameterEvidence
	require.NoError(t, DB.Where("model_name = ? AND field = ?", "deepwl/gpt-image-2-all", "public_price").First(&priceEvidence).Error)
	assert.Equal(t, "https://zx1.deepwl.net/api/pricing", priceEvidence.SourceURL)
	assert.Contains(t, priceEvidence.Notes, "not runtime routing")
	priceEvidence.VerificationStatus = ModelOperationEvidenceStatusTested
	priceEvidence.VerifiedAt = 1784999999
	priceEvidence.Notes = "Administrator verified this evidence against the upstream response."
	require.NoError(t, SaveModelOperationParameterEvidence(&priceEvidence))

	require.NoError(t, SeedDefaultModelOperationParameterEvidence())
	var total int64
	require.NoError(t, DB.Model(&ModelOperationParameterEvidence{}).Where("model_name IN ?", models).Count(&total).Error)
	assert.EqualValues(t, 18, total)
	priceEvidence = ModelOperationParameterEvidence{}
	require.NoError(t, DB.Where("model_name = ? AND field = ?", "deepwl/gpt-image-2-all", "public_price").First(&priceEvidence).Error)
	assert.Equal(t, ModelOperationEvidenceStatusTested, priceEvidence.VerificationStatus)
	assert.EqualValues(t, 1784999999, priceEvidence.VerifiedAt)
	assert.Equal(t, "Administrator verified this evidence against the upstream response.", priceEvidence.Notes)
}

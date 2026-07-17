package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	ModelOperationEvidenceSourceDoc    = "doc"
	ModelOperationEvidenceSourceDemo   = "demo"
	ModelOperationEvidenceSourceManual = "manual"
	ModelOperationEvidenceSourceTest   = "test"

	ModelOperationEvidenceStatusUnverified = "unverified"
	ModelOperationEvidenceStatusDocumented = "documented"
	ModelOperationEvidenceStatusTested     = "tested"
	ModelOperationEvidenceStatusRejected   = "rejected"
)

var (
	ErrModelOperationBindingConflict           = errors.New("model operation binding contract conflict")
	ErrModelOperationParameterEvidenceConflict = errors.New("model operation parameter evidence ownership conflict")
)

type ModelOperationBindingRevision struct {
	Id              int    `json:"id"`
	BindingId       int    `json:"binding_id" gorm:"not null;index"`
	ModelName       string `json:"model_name" gorm:"type:varchar(255);not null;uniqueIndex:uk_model_operation_revision,priority:1;index"`
	Operation       string `json:"operation" gorm:"type:varchar(64);not null;uniqueIndex:uk_model_operation_revision,priority:2;index"`
	Revision        int    `json:"revision" gorm:"not null;uniqueIndex:uk_model_operation_revision,priority:3"`
	ProfileKey      string `json:"profile_key" gorm:"type:varchar(128);not null"`
	ProfileVersion  int    `json:"profile_version" gorm:"not null"`
	ContractVersion int    `json:"contract_version" gorm:"not null"`
	ContractHash    string `json:"contract_hash" gorm:"type:varchar(64);not null;index"`
	Overrides       string `json:"overrides" gorm:"type:text;not null"`
	Enabled         bool   `json:"enabled" gorm:"not null"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint;not null"`
}

type ModelOperationParameterEvidence struct {
	Id                 int    `json:"id"`
	ModelName          string `json:"model_name" gorm:"type:varchar(255);not null;index:idx_model_operation_parameter,priority:1"`
	Operation          string `json:"operation" gorm:"type:varchar(64);not null;index:idx_model_operation_parameter,priority:2"`
	Field              string `json:"field" gorm:"type:varchar(128);not null;index:idx_model_operation_parameter,priority:3"`
	SourceType         string `json:"source_type" gorm:"type:varchar(16);not null;index"`
	SourceURL          string `json:"source_url,omitempty" gorm:"type:text"`
	SourceLocator      string `json:"source_locator,omitempty" gorm:"type:text"`
	VerificationStatus string `json:"verification_status" gorm:"type:varchar(16);not null;index"`
	VerifiedAt         int64  `json:"verified_at,omitempty" gorm:"bigint"`
	Notes              string `json:"notes,omitempty" gorm:"type:text"`
	CreatedTime        int64  `json:"created_time" gorm:"bigint;not null"`
	UpdatedTime        int64  `json:"updated_time" gorm:"bigint;not null"`
}

func appendModelOperationBindingRevisionIfChanged(tx *gorm.DB, binding *ModelOperationBinding, now int64) error {
	var latest ModelOperationBindingRevision
	err := tx.Where("model_name = ? AND operation = ?", binding.ModelName, binding.Operation).
		Order("revision DESC").First(&latest).Error
	nextRevision := 1
	if err == nil {
		if latest.ContractHash == binding.ContractHash && latest.Enabled == binding.Enabled {
			return nil
		}
		nextRevision = latest.Revision + 1
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	revision := ModelOperationBindingRevision{
		BindingId:       binding.Id,
		ModelName:       binding.ModelName,
		Operation:       binding.Operation,
		Revision:        nextRevision,
		ProfileKey:      binding.ProfileKey,
		ProfileVersion:  binding.ProfileVersion,
		ContractVersion: binding.ContractVersion,
		ContractHash:    binding.ContractHash,
		Overrides:       binding.Overrides,
		Enabled:         binding.Enabled,
		CreatedTime:     now,
	}
	return tx.Create(&revision).Error
}

func GetModelOperationBindingRevisions(modelName string, operation string, offset int, limit int) ([]ModelOperationBindingRevision, int64, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || len(modelName) > 255 {
		return nil, 0, errors.New("model_name is required and must be 255 characters or fewer")
	}
	normalizedOperation, err := normalizeContractIdentifier(operation, 64)
	if err != nil {
		return nil, 0, fmt.Errorf("operation: %w", err)
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := DB.Model(&ModelOperationBindingRevision{}).
		Where("model_name = ? AND operation = ?", modelName, normalizedOperation)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var revisions []ModelOperationBindingRevision
	if err := query.Order("revision DESC").Offset(offset).Limit(limit).Find(&revisions).Error; err != nil {
		return nil, 0, err
	}
	return revisions, total, nil
}

func RollbackModelOperationBinding(modelName string, operation string, revision int, expectedContractHash string) (*ModelOperationBinding, error) {
	modelName = strings.TrimSpace(modelName)
	normalizedOperation, err := normalizeContractIdentifier(operation, 64)
	if err != nil {
		return nil, fmt.Errorf("operation: %w", err)
	}
	if modelName == "" || len(modelName) > 255 {
		return nil, errors.New("model_name is required and must be 255 characters or fewer")
	}
	if revision <= 0 {
		return nil, errors.New("revision must be greater than zero")
	}
	var target ModelOperationBindingRevision
	if err := DB.Where("model_name = ? AND operation = ? AND revision = ?", modelName, normalizedOperation, revision).
		First(&target).Error; err != nil {
		return nil, err
	}
	binding := ModelOperationBinding{
		ModelName:      target.ModelName,
		Operation:      target.Operation,
		ProfileKey:     target.ProfileKey,
		ProfileVersion: target.ProfileVersion,
		Overrides:      target.Overrides,
		Enabled:        target.Enabled,
	}
	if err := saveModelOperationBinding(&binding, &expectedContractHash); err != nil {
		return nil, err
	}
	return &binding, nil
}

func normalizeModelOperationParameterEvidence(evidence *ModelOperationParameterEvidence) error {
	if evidence == nil {
		return errors.New("parameter evidence is required")
	}
	evidence.ModelName = strings.TrimSpace(evidence.ModelName)
	if evidence.ModelName == "" || len(evidence.ModelName) > 255 {
		return errors.New("model_name is required and must be 255 characters or fewer")
	}
	operation, err := normalizeContractIdentifier(evidence.Operation, 64)
	if err != nil {
		return fmt.Errorf("operation: %w", err)
	}
	field, err := normalizeContractIdentifier(evidence.Field, 128)
	if err != nil {
		return fmt.Errorf("field: %w", err)
	}
	sourceType := strings.ToLower(strings.TrimSpace(evidence.SourceType))
	switch sourceType {
	case ModelOperationEvidenceSourceDoc, ModelOperationEvidenceSourceDemo, ModelOperationEvidenceSourceManual, ModelOperationEvidenceSourceTest:
	default:
		return fmt.Errorf("unsupported evidence source_type %q", evidence.SourceType)
	}
	verificationStatus := strings.ToLower(strings.TrimSpace(evidence.VerificationStatus))
	switch verificationStatus {
	case ModelOperationEvidenceStatusUnverified, ModelOperationEvidenceStatusDocumented, ModelOperationEvidenceStatusTested, ModelOperationEvidenceStatusRejected:
	default:
		return fmt.Errorf("unsupported verification_status %q", evidence.VerificationStatus)
	}
	if evidence.VerifiedAt < 0 {
		return errors.New("verified_at must not be negative")
	}
	evidence.Operation = operation
	evidence.Field = field
	evidence.SourceType = sourceType
	evidence.SourceURL = strings.TrimSpace(evidence.SourceURL)
	evidence.SourceLocator = strings.TrimSpace(evidence.SourceLocator)
	evidence.VerificationStatus = verificationStatus
	evidence.Notes = strings.TrimSpace(evidence.Notes)
	return nil
}

func SaveModelOperationParameterEvidence(evidence *ModelOperationParameterEvidence) error {
	if err := normalizeModelOperationParameterEvidence(evidence); err != nil {
		return err
	}
	now := common.GetTimestamp()
	if evidence.Id <= 0 {
		evidence.CreatedTime = now
		evidence.UpdatedTime = now
		return DB.Create(evidence).Error
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var stored ModelOperationParameterEvidence
		if err := lockForUpdate(tx).Where("id = ?", evidence.Id).First(&stored).Error; err != nil {
			return err
		}
		if stored.ModelName != evidence.ModelName || stored.Operation != evidence.Operation {
			return fmt.Errorf("%w: model_name and operation cannot change for existing evidence", ErrModelOperationParameterEvidenceConflict)
		}
		evidence.CreatedTime = stored.CreatedTime
		evidence.UpdatedTime = now
		return tx.Model(&stored).Select(
			"model_name", "operation", "field", "source_type", "source_url", "source_locator",
			"verification_status", "verified_at", "notes", "updated_time",
		).Updates(evidence).Error
	})
}

func GetModelOperationParameterEvidence(modelName string, operation string, field string, offset int, limit int) ([]ModelOperationParameterEvidence, int64, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || len(modelName) > 255 {
		return nil, 0, errors.New("model_name is required and must be 255 characters or fewer")
	}
	query := DB.Model(&ModelOperationParameterEvidence{}).Where("model_name = ?", modelName)
	if strings.TrimSpace(operation) != "" {
		normalizedOperation, err := normalizeContractIdentifier(operation, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("operation: %w", err)
		}
		query = query.Where("operation = ?", normalizedOperation)
	}
	if strings.TrimSpace(field) != "" {
		normalizedField, err := normalizeContractIdentifier(field, 128)
		if err != nil {
			return nil, 0, fmt.Errorf("field: %w", err)
		}
		query = query.Where("field = ?", normalizedField)
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []ModelOperationParameterEvidence
	if err := query.Order("operation ASC, field ASC, id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func DeleteModelOperationParameterEvidence(id int) error {
	if id <= 0 {
		return errors.New("evidence id must be greater than zero")
	}
	result := DB.Delete(&ModelOperationParameterEvidence{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

const defaultModelOperationEvidenceObservedAt int64 = 1784131200

type defaultModelOperationEvidenceSeed struct {
	ModelName  string
	Operation  string
	Field      string
	SourceType string
	SourceURL  string
	Source     string
	Notes      string
}

// SeedDefaultModelOperationParameterEvidence records the three provenance
// layers used by the core DeepWL contracts. These are documented observations,
// not claims that a paid upstream request was executed by the seed process.
func SeedDefaultModelOperationParameterEvidence() error {
	if DB == nil || !DB.Migrator().HasTable(&ModelOperationParameterEvidence{}) {
		return nil
	}
	const (
		gptDocURL        = "https://doc.deepwl.cn/zh/images/gpt-image-2/generation.md"
		omniDocURL       = "https://doc.deepwl.cn/zh/videos/omni/omni-fast.md"
		omniV2VDocURL    = "https://doc.deepwl.cn/zh/videos/omni/omni-fast-v2v.md"
		demoConfigURL    = "https://demo.duoyuanx.com/api/generation/config"
		tapLaterURL      = "https://cjzz.top"
		publicPricingURL = "https://zx1.deepwl.net/api/pricing"
	)
	seeds := make([]defaultModelOperationEvidenceSeed, 0, 24)
	addLayers := func(modelName, operation, docURL, documented, demo, release string) {
		seeds = append(seeds,
			defaultModelOperationEvidenceSeed{modelName, operation, "documented_capability", ModelOperationEvidenceSourceDoc, docURL, "official documentation", documented},
			defaultModelOperationEvidenceSeed{modelName, operation, "demo_allowlist", ModelOperationEvidenceSourceDemo, demoConfigURL, "Duoyuanx generation config", demo},
			defaultModelOperationEvidenceSeed{modelName, operation, "taplater_release", ModelOperationEvidenceSourceManual, tapLaterURL, "TapLater published contract observation", release},
		)
	}
	addLayers(
		"deepwl/gpt-image-2", "image.generate", gptDocURL,
		"DeepWL documents URL/base64 image responses and common size/quality/count controls.",
		"Demo exposes 13 sizes, quality low/medium/high, count 1-4, and up to 8 reference images.",
		"TapLater release exposes 13 sizes, quality low/medium/high, response_format url/b64_json, and fixes n=1.",
	)
	addLayers(
		"deepwl/gpt-image-2-c", "image.generate", gptDocURL,
		"DeepWL documents URL/base64 image responses and common size/quality/count controls.",
		"Demo exposes 13 sizes, quality low/medium/high, count 1-4, and up to 8 reference images.",
		"TapLater release shares the GPT Image 2 contract: 13 sizes, quality low/medium/high, response_format url/b64_json, n=1.",
	)
	addLayers(
		"deepwl/gpt-image-2-all", "image.generate", gptDocURL,
		"DeepWL documents URL/base64 image responses and common size/quality/count controls.",
		"Demo exposes three sizes (1024x1024, 1536x1024, 1024x1536), quality low/medium/high, count 1-4.",
		"TapLater release limits this alias to the three Demo sizes, quality low/medium/high, response_format url/b64_json, n=1.",
	)
	addLayers(
		"deepwl/omni-fast", "video.generate", omniDocURL,
		"DeepWL Omni Fast documentation describes asynchronous video generation with duration, ratio, resolution, and optional images.",
		"Demo allowlist exposes 4/6/8/10 seconds, 720p, five ratios, and up to five reference images.",
		"TapLater release uses 4/6/8/10 seconds, 720p, five ratios, and up to five reference images.",
	)
	addLayers(
		"deepwl/omni-fast-v2v", "video.generate", omniV2VDocURL,
		"DeepWL Omni Fast V2V documentation describes asynchronous video-to-video generation with a required reference video.",
		"Demo allowlist exposes 4/6/8/10 seconds, 720p, five ratios, one reference video, and optional images.",
		"TapLater release requires one public MP4 video under 15MB, permits up to five reference images, and uses 4/6/8/10 seconds at 720p.",
	)
	for _, price := range []struct {
		model string
		value string
	}{
		{"deepwl/gpt-image-2", "public price evidence: ¥0.15"},
		{"deepwl/gpt-image-2-c", "public price evidence: ¥0.10"},
		{"deepwl/gpt-image-2-all", "public price evidence: ¥0.08"},
	} {
		seeds = append(seeds, defaultModelOperationEvidenceSeed{
			ModelName:  price.model,
			Operation:  "image.generate",
			Field:      "public_price",
			SourceType: ModelOperationEvidenceSourceDoc,
			SourceURL:  publicPricingURL,
			Source:     "DeepWL public pricing endpoint",
			Notes:      price.value + "; public price evidence only, not runtime routing or actual settlement proof.",
		})
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		for _, seed := range seeds {
			evidence := &ModelOperationParameterEvidence{
				ModelName:          seed.ModelName,
				Operation:          seed.Operation,
				Field:              seed.Field,
				SourceType:         seed.SourceType,
				SourceURL:          seed.SourceURL,
				SourceLocator:      seed.Source,
				VerificationStatus: ModelOperationEvidenceStatusDocumented,
				VerifiedAt:         defaultModelOperationEvidenceObservedAt,
				Notes:              seed.Notes,
			}
			if err := normalizeModelOperationParameterEvidence(evidence); err != nil {
				return err
			}
			var stored ModelOperationParameterEvidence
			err := lockForUpdate(tx).Where(
				"model_name = ? AND operation = ? AND field = ? AND source_type = ? AND source_url = ? AND source_locator = ?",
				evidence.ModelName, evidence.Operation, evidence.Field, evidence.SourceType, evidence.SourceURL, evidence.SourceLocator,
			).First(&stored).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				now := common.GetTimestamp()
				evidence.CreatedTime = now
				evidence.UpdatedTime = now
				if err := tx.Create(evidence).Error; err != nil {
					return err
				}
			case err != nil:
				return err
			}
		}
		return nil
	})
}

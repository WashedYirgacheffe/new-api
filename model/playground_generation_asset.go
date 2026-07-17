package model

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	PlaygroundGenerationMaximumAssets         = 16
	PlaygroundGenerationMaximumAssetBytes     = 20 * 1024 * 1024
	PlaygroundGenerationMaximumUserAssetBytes = 200 * 1024 * 1024
)

var (
	ErrPlaygroundGenerationAssetNotFound = errors.New("playground generation asset not found")
	ErrPlaygroundGenerationAssetConflict = errors.New("playground generation asset ordinal already exists")
	ErrPlaygroundGenerationAssetLimit    = errors.New("playground generation asset storage limit reached")
)

type PlaygroundGenerationAsset struct {
	Id           int    `json:"-"`
	GenerationId string `json:"generation_id" gorm:"type:varchar(32);not null;uniqueIndex:idx_pg_generation_asset_ordinal,priority:1;index"`
	UserId       int    `json:"-" gorm:"not null;index;uniqueIndex:idx_pg_generation_asset_ordinal,priority:2"`
	Ordinal      int    `json:"ordinal" gorm:"not null;uniqueIndex:idx_pg_generation_asset_ordinal,priority:3"`
	MimeType     string `json:"mime_type" gorm:"type:varchar(64);not null"`
	SizeBytes    int64  `json:"size_bytes" gorm:"not null"`
	SHA256       string `json:"-" gorm:"type:varchar(64);not null"`
	RelativePath string `json:"-" gorm:"type:varchar(512);not null"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;not null"`
}

type PlaygroundGenerationAssetCreate struct {
	GenerationId string
	Ordinal      int
	MimeType     string
	SizeBytes    int64
	SHA256       string
	RelativePath string
}

func validatePlaygroundGenerationAssetCreate(input PlaygroundGenerationAssetCreate) (PlaygroundGenerationAssetCreate, error) {
	id, err := normalizePlaygroundGenerationId(input.GenerationId)
	if err != nil {
		return input, err
	}
	input.GenerationId = id
	if input.Ordinal < 0 || input.Ordinal >= PlaygroundGenerationMaximumAssets {
		return input, fmt.Errorf("ordinal must be between 0 and %d", PlaygroundGenerationMaximumAssets-1)
	}
	input.MimeType = strings.ToLower(strings.TrimSpace(input.MimeType))
	if input.MimeType == "" || len(input.MimeType) > 64 {
		return input, errors.New("mime_type is required and must be 64 characters or fewer")
	}
	if input.SizeBytes <= 0 || input.SizeBytes > PlaygroundGenerationMaximumAssetBytes {
		return input, fmt.Errorf("asset must be between 1 and %d bytes", PlaygroundGenerationMaximumAssetBytes)
	}
	input.SHA256 = strings.ToLower(strings.TrimSpace(input.SHA256))
	if len(input.SHA256) != 64 {
		return input, errors.New("sha256 must be a 64-character hexadecimal string")
	}
	if _, err := hex.DecodeString(input.SHA256); err != nil {
		return input, errors.New("sha256 must be a 64-character hexadecimal string")
	}
	input.RelativePath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(input.RelativePath)))
	if input.RelativePath == "." || input.RelativePath == "" || filepath.IsAbs(input.RelativePath) || strings.HasPrefix(input.RelativePath, "../") {
		return input, errors.New("relative_path must stay inside the configured asset directory")
	}
	return input, nil
}

func SavePlaygroundGenerationAsset(userId int, input PlaygroundGenerationAssetCreate) (*PlaygroundGenerationAsset, error) {
	if userId <= 0 {
		return nil, errors.New("user id must be greater than zero")
	}
	normalized, err := validatePlaygroundGenerationAssetCreate(input)
	if err != nil {
		return nil, err
	}
	var result PlaygroundGenerationAsset
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockPlaygroundGenerationUser(tx, userId); err != nil {
			return err
		}
		var generation PlaygroundGeneration
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", normalized.GenerationId, userId).First(&generation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPlaygroundGenerationNotFound
			}
			return err
		}
		if generation.Status != PlaygroundGenerationStatusPending {
			return ErrPlaygroundGenerationFinalized
		}

		var existing PlaygroundGenerationAsset
		err := tx.Where("generation_id = ? AND user_id = ? AND ordinal = ?", normalized.GenerationId, userId, normalized.Ordinal).First(&existing).Error
		if err == nil {
			if existing.SHA256 == normalized.SHA256 {
				result = existing
				return nil
			}
			return ErrPlaygroundGenerationAssetConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var usedBytes int64
		if err := tx.Model(&PlaygroundGenerationAsset{}).Where("user_id = ?", userId).Select("COALESCE(SUM(size_bytes), 0)").Scan(&usedBytes).Error; err != nil {
			return err
		}
		if usedBytes < 0 || usedBytes > PlaygroundGenerationMaximumUserAssetBytes-normalized.SizeBytes {
			return ErrPlaygroundGenerationAssetLimit
		}
		result = PlaygroundGenerationAsset{
			GenerationId: normalized.GenerationId,
			UserId:       userId,
			Ordinal:      normalized.Ordinal,
			MimeType:     normalized.MimeType,
			SizeBytes:    normalized.SizeBytes,
			SHA256:       normalized.SHA256,
			RelativePath: normalized.RelativePath,
			CreatedAt:    common.GetTimestamp(),
		}
		return tx.Create(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func CountPlaygroundGenerationAssets(userId int, rawGenerationId string) (int64, error) {
	if userId <= 0 {
		return 0, errors.New("user id must be greater than zero")
	}
	id, err := normalizePlaygroundGenerationId(rawGenerationId)
	if err != nil {
		return 0, err
	}
	var total int64
	if err := DB.Model(&PlaygroundGenerationAsset{}).Where("generation_id = ? AND user_id = ?", id, userId).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func GetPlaygroundGenerationAsset(userId int, rawGenerationId string, ordinal int) (*PlaygroundGenerationAsset, error) {
	if userId <= 0 {
		return nil, errors.New("user id must be greater than zero")
	}
	id, err := normalizePlaygroundGenerationId(rawGenerationId)
	if err != nil {
		return nil, err
	}
	var asset PlaygroundGenerationAsset
	if err := DB.Where("generation_id = ? AND user_id = ? AND ordinal = ?", id, userId, ordinal).First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlaygroundGenerationAssetNotFound
		}
		return nil, err
	}
	return &asset, nil
}

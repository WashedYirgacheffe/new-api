package model

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	PlaygroundGenerationOperationImage = "image"
	PlaygroundGenerationOperationVideo = "video"

	PlaygroundGenerationStatusPending   = "pending"
	PlaygroundGenerationStatusSucceeded = "succeeded"
	PlaygroundGenerationStatusFailed    = "failed"

	playgroundGenerationMaxPromptLength    = 20000
	playgroundGenerationMaxErrorLength     = 4096
	playgroundGenerationMaxParameters      = 128
	playgroundGenerationMaxJSONBytes       = 65_535
	playgroundGenerationMaxOutputs         = 16
	playgroundGenerationMaxOutputURLLength = 2048
	playgroundGenerationMaxQuotedQuota     = math.MaxInt32
	playgroundGenerationMaxQuotedAmount    = 1_000_000_000
	playgroundGenerationDefaultPageSize    = 20
	playgroundGenerationMaximumPageSize    = 100
	playgroundGenerationMaximumPerUser     = 500
)

var (
	ErrPlaygroundGenerationNotFound  = errors.New("playground generation not found")
	ErrPlaygroundGenerationFinalized = errors.New("playground generation is already finalized")
	ErrPlaygroundGenerationLimit     = errors.New("playground generation history limit reached")
)

// PlaygroundGeneration is a user-owned history record. Billing and settlement
// remain authoritative in the existing billing tables; quote fields here are
// only the snapshot shown to the user when the generation was submitted.
type PlaygroundGeneration struct {
	Id              string                 `json:"id" gorm:"type:varchar(32);primaryKey"`
	UserId          int                    `json:"-" gorm:"not null;index:idx_pg_generation_user_operation_created,priority:1"`
	Operation       string                 `json:"operation" gorm:"type:varchar(16);not null;index:idx_pg_generation_user_operation_created,priority:2"`
	Model           string                 `json:"model" gorm:"type:varchar(255);not null"`
	Group           string                 `json:"group" gorm:"type:varchar(64);not null"`
	Prompt          string                 `json:"prompt" gorm:"type:text;not null"`
	ParametersJSON  string                 `json:"-" gorm:"column:parameters;type:text;not null"`
	OutputsJSON     string                 `json:"-" gorm:"column:outputs;type:text;not null"`
	Parameters      map[string]interface{} `json:"parameters" gorm:"-"`
	Outputs         []string               `json:"outputs" gorm:"-"`
	TaskId          string                 `json:"task_id" gorm:"type:varchar(191);not null"`
	Status          string                 `json:"status" gorm:"type:varchar(16);not null"`
	Error           string                 `json:"error" gorm:"type:text;not null"`
	ContractHash    string                 `json:"contract_hash" gorm:"type:varchar(128);not null"`
	ContractVersion int                    `json:"contract_version" gorm:"not null"`
	PricingVersion  string                 `json:"pricing_version" gorm:"type:varchar(128);not null"`
	QuotedQuota     int                    `json:"quoted_quota" gorm:"not null"`
	Amount          float64                `json:"amount" gorm:"not null"`
	CreatedAt       int64                  `json:"created_at" gorm:"bigint;not null;index:idx_pg_generation_user_operation_created,priority:3"`
	UpdatedAt       int64                  `json:"updated_at" gorm:"bigint;not null"`
	CompletedAt     int64                  `json:"completed_at" gorm:"bigint;not null"`
}

type PlaygroundGenerationCreate struct {
	Operation       string
	Model           string
	Group           string
	Prompt          string
	Parameters      map[string]interface{}
	Outputs         []string
	TaskId          string
	Status          string
	Error           string
	ContractHash    string
	ContractVersion int
	PricingVersion  string
	QuotedQuota     int
	Amount          float64
}

type PlaygroundGenerationUpdate struct {
	Outputs *[]string
	TaskId  *string
	Status  *string
	Error   *string
}

func normalizePlaygroundGenerationOperation(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case PlaygroundGenerationOperationImage, PlaygroundGenerationOperationVideo:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported playground generation operation %q", value)
	}
}

func normalizePlaygroundGenerationStatus(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = PlaygroundGenerationStatusPending
	}
	switch value {
	case PlaygroundGenerationStatusPending, PlaygroundGenerationStatusSucceeded, PlaygroundGenerationStatusFailed:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported playground generation status %q", value)
	}
}

func normalizePlaygroundGenerationId(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) != 32 {
		return "", errors.New("playground generation id must be a 32-character hexadecimal string")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", errors.New("playground generation id must be a 32-character hexadecimal string")
	}
	return strings.ToLower(value), nil
}

func normalizePlaygroundGenerationOutputs(values []string) ([]string, error) {
	if len(values) > playgroundGenerationMaxOutputs {
		return nil, fmt.Errorf("outputs must contain %d entries or fewer", playgroundGenerationMaxOutputs)
	}
	result := make([]string, 0, len(values))
	for index, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > playgroundGenerationMaxOutputURLLength {
			return nil, fmt.Errorf("outputs[%d] must be a non-empty URL with %d characters or fewer", index, playgroundGenerationMaxOutputURLLength)
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" || parsed.User != nil {
			return nil, fmt.Errorf("outputs[%d] must be an absolute HTTP(S) URL", index)
		}
		scheme := strings.ToLower(parsed.Scheme)
		if scheme != "http" && scheme != "https" {
			return nil, fmt.Errorf("outputs[%d] must be an absolute HTTP(S) URL", index)
		}
		result = append(result, value)
	}
	return result, nil
}

func encodePlaygroundGenerationParameters(parameters map[string]interface{}) (string, map[string]interface{}, error) {
	if parameters == nil {
		parameters = map[string]interface{}{}
	}
	if len(parameters) > playgroundGenerationMaxParameters {
		return "", nil, fmt.Errorf("parameters must contain %d entries or fewer", playgroundGenerationMaxParameters)
	}
	encoded, err := common.Marshal(parameters)
	if err != nil {
		return "", nil, fmt.Errorf("parameters must be valid JSON: %w", err)
	}
	if len(encoded) > playgroundGenerationMaxJSONBytes {
		return "", nil, fmt.Errorf("parameters must be %d bytes or fewer", playgroundGenerationMaxJSONBytes)
	}
	return string(encoded), parameters, nil
}

func encodePlaygroundGenerationOutputs(outputs []string) (string, []string, error) {
	normalized, err := normalizePlaygroundGenerationOutputs(outputs)
	if err != nil {
		return "", nil, err
	}
	if normalized == nil {
		normalized = []string{}
	}
	encoded, err := common.Marshal(normalized)
	if err != nil {
		return "", nil, fmt.Errorf("outputs must be valid JSON: %w", err)
	}
	if len(encoded) > playgroundGenerationMaxJSONBytes {
		return "", nil, fmt.Errorf("outputs must be %d bytes or fewer", playgroundGenerationMaxJSONBytes)
	}
	return string(encoded), normalized, nil
}

func lockPlaygroundGenerationUser(tx *gorm.DB, userId int) error {
	var user User
	if err := lockForUpdate(tx).Select("id").Where("id = ?", userId).Take(&user).Error; err != nil {
		return fmt.Errorf("lock playground generation user: %w", err)
	}
	return nil
}

func hydratePlaygroundGeneration(generation *PlaygroundGeneration) error {
	parameters := map[string]interface{}{}
	if err := common.UnmarshalJsonStr(generation.ParametersJSON, &parameters); err != nil {
		return fmt.Errorf("decode playground generation parameters: %w", err)
	}
	outputs := []string{}
	if err := common.UnmarshalJsonStr(generation.OutputsJSON, &outputs); err != nil {
		return fmt.Errorf("decode playground generation outputs: %w", err)
	}
	generation.Parameters = parameters
	generation.Outputs = outputs
	return nil
}

func validatePlaygroundGenerationLifecycle(generation *PlaygroundGeneration) error {
	status, err := normalizePlaygroundGenerationStatus(generation.Status)
	if err != nil {
		return err
	}
	generation.Status = status
	generation.Error = strings.TrimSpace(generation.Error)
	if len(generation.Error) > playgroundGenerationMaxErrorLength {
		return fmt.Errorf("error must be %d characters or fewer", playgroundGenerationMaxErrorLength)
	}
	switch status {
	case PlaygroundGenerationStatusPending:
		if len(generation.Outputs) != 0 || generation.Error != "" {
			return errors.New("pending generations must not contain outputs or an error")
		}
	case PlaygroundGenerationStatusSucceeded:
		if generation.Error != "" {
			return errors.New("succeeded generations must not contain an error")
		}
	case PlaygroundGenerationStatusFailed:
		if generation.Error == "" {
			return errors.New("failed generations must contain an error")
		}
		if len(generation.Outputs) != 0 {
			return errors.New("failed generations must not contain outputs")
		}
	}
	return nil
}

func normalizePlaygroundGenerationCreate(userId int, input PlaygroundGenerationCreate) (*PlaygroundGeneration, error) {
	if userId <= 0 {
		return nil, errors.New("user id must be greater than zero")
	}
	operation, err := normalizePlaygroundGenerationOperation(input.Operation)
	if err != nil {
		return nil, err
	}
	modelName := strings.TrimSpace(input.Model)
	if modelName == "" || len(modelName) > 255 {
		return nil, errors.New("model is required and must be 255 characters or fewer")
	}
	group := strings.TrimSpace(input.Group)
	if group == "" || len(group) > 64 {
		return nil, errors.New("group is required and must be 64 characters or fewer")
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" || len(prompt) > playgroundGenerationMaxPromptLength {
		return nil, fmt.Errorf("prompt is required and must be %d characters or fewer", playgroundGenerationMaxPromptLength)
	}
	parametersJSON, parameters, err := encodePlaygroundGenerationParameters(input.Parameters)
	if err != nil {
		return nil, err
	}
	outputsJSON, outputs, err := encodePlaygroundGenerationOutputs(input.Outputs)
	if err != nil {
		return nil, err
	}
	taskId := strings.TrimSpace(input.TaskId)
	if len(taskId) > 191 {
		return nil, errors.New("task_id must be 191 characters or fewer")
	}
	contractHash := strings.TrimSpace(input.ContractHash)
	if len(contractHash) > 128 {
		return nil, errors.New("contract_hash must be 128 characters or fewer")
	}
	pricingVersion := strings.TrimSpace(input.PricingVersion)
	if len(pricingVersion) > 128 {
		return nil, errors.New("pricing_version must be 128 characters or fewer")
	}
	if input.ContractVersion < 0 {
		return nil, errors.New("contract_version must not be negative")
	}
	if input.QuotedQuota < 0 || input.QuotedQuota > playgroundGenerationMaxQuotedQuota {
		return nil, fmt.Errorf("quoted_quota must be between 0 and %d", playgroundGenerationMaxQuotedQuota)
	}
	if math.IsNaN(input.Amount) || math.IsInf(input.Amount, 0) || input.Amount < 0 || input.Amount > playgroundGenerationMaxQuotedAmount {
		return nil, fmt.Errorf("amount must be between 0 and %d", playgroundGenerationMaxQuotedAmount)
	}
	now := common.GetTimestamp()
	generation := &PlaygroundGeneration{
		Id:              common.GetUUID(),
		UserId:          userId,
		Operation:       operation,
		Model:           modelName,
		Group:           group,
		Prompt:          prompt,
		ParametersJSON:  parametersJSON,
		OutputsJSON:     outputsJSON,
		Parameters:      parameters,
		Outputs:         outputs,
		TaskId:          taskId,
		Status:          input.Status,
		Error:           input.Error,
		ContractHash:    contractHash,
		ContractVersion: input.ContractVersion,
		PricingVersion:  pricingVersion,
		QuotedQuota:     input.QuotedQuota,
		Amount:          input.Amount,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := validatePlaygroundGenerationLifecycle(generation); err != nil {
		return nil, err
	}
	if generation.Status != PlaygroundGenerationStatusPending {
		generation.CompletedAt = now
	}
	return generation, nil
}

func CreatePlaygroundGeneration(userId int, input PlaygroundGenerationCreate) (*PlaygroundGeneration, error) {
	generation, err := normalizePlaygroundGenerationCreate(userId, input)
	if err != nil {
		return nil, err
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockPlaygroundGenerationUser(tx, userId); err != nil {
			return err
		}
		var total int64
		if err := tx.Model(&PlaygroundGeneration{}).Where("user_id = ?", userId).Count(&total).Error; err != nil {
			return err
		}
		if total >= playgroundGenerationMaximumPerUser {
			return ErrPlaygroundGenerationLimit
		}
		return tx.Create(generation).Error
	})
	if err != nil {
		return nil, err
	}
	return generation, nil
}

func ListPlaygroundGenerations(userId int, operation string, offset int, limit int) ([]PlaygroundGeneration, int64, error) {
	if userId <= 0 {
		return nil, 0, errors.New("user id must be greater than zero")
	}
	normalizedOperation, err := normalizePlaygroundGenerationOperation(operation)
	if err != nil {
		return nil, 0, err
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = playgroundGenerationDefaultPageSize
	}
	if limit > playgroundGenerationMaximumPageSize {
		limit = playgroundGenerationMaximumPageSize
	}
	query := DB.Model(&PlaygroundGeneration{}).
		Where("user_id = ? AND operation = ?", userId, normalizedOperation)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]PlaygroundGeneration, 0)
	if err := query.Order("created_at DESC").Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	for index := range items {
		if err := hydratePlaygroundGeneration(&items[index]); err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}

func GetPlaygroundGenerationById(userId int, rawId string) (*PlaygroundGeneration, error) {
	if userId <= 0 {
		return nil, errors.New("user id must be greater than zero")
	}
	id, err := normalizePlaygroundGenerationId(rawId)
	if err != nil {
		return nil, err
	}
	var generation PlaygroundGeneration
	if err := DB.Where("id = ? AND user_id = ?", id, userId).First(&generation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlaygroundGenerationNotFound
		}
		return nil, err
	}
	if err := hydratePlaygroundGeneration(&generation); err != nil {
		return nil, err
	}
	return &generation, nil
}

func UpdatePlaygroundGeneration(userId int, rawId string, update PlaygroundGenerationUpdate) (*PlaygroundGeneration, error) {
	if userId <= 0 {
		return nil, errors.New("user id must be greater than zero")
	}
	id, err := normalizePlaygroundGenerationId(rawId)
	if err != nil {
		return nil, err
	}
	if update.Outputs == nil && update.TaskId == nil && update.Status == nil && update.Error == nil {
		return nil, errors.New("at least one lifecycle field is required")
	}
	var result PlaygroundGeneration
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", id, userId).First(&result).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPlaygroundGenerationNotFound
			}
			return err
		}
		if err := hydratePlaygroundGeneration(&result); err != nil {
			return err
		}
		previousStatus := result.Status
		previousTaskId := result.TaskId
		previousError := result.Error
		previousOutputs := slices.Clone(result.Outputs)

		if update.Outputs != nil {
			outputsJSON, outputs, err := encodePlaygroundGenerationOutputs(*update.Outputs)
			if err != nil {
				return err
			}
			result.OutputsJSON = outputsJSON
			result.Outputs = outputs
		}
		if update.TaskId != nil {
			result.TaskId = strings.TrimSpace(*update.TaskId)
			if len(result.TaskId) > 191 {
				return errors.New("task_id must be 191 characters or fewer")
			}
		}
		if update.Status != nil {
			result.Status = *update.Status
		}
		if update.Error != nil {
			result.Error = *update.Error
		}
		if previousStatus != PlaygroundGenerationStatusPending {
			unchanged := previousStatus == result.Status && previousTaskId == result.TaskId &&
				previousError == result.Error && slices.Equal(previousOutputs, result.Outputs)
			if unchanged {
				return nil
			}
			return ErrPlaygroundGenerationFinalized
		}
		if err := validatePlaygroundGenerationLifecycle(&result); err != nil {
			return err
		}

		now := common.GetTimestamp()
		result.UpdatedAt = now
		if result.Status == PlaygroundGenerationStatusPending {
			result.CompletedAt = 0
		} else {
			result.CompletedAt = now
		}
		updateResult := tx.Model(&PlaygroundGeneration{}).
			Where("id = ? AND user_id = ? AND status = ?", id, userId, PlaygroundGenerationStatusPending).
			Updates(map[string]interface{}{
				"outputs":      result.OutputsJSON,
				"task_id":      result.TaskId,
				"status":       result.Status,
				"error":        result.Error,
				"updated_at":   result.UpdatedAt,
				"completed_at": result.CompletedAt,
			})
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			var current PlaygroundGeneration
			if err := tx.Where("id = ? AND user_id = ?", id, userId).First(&current).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrPlaygroundGenerationNotFound
				}
				return err
			}
			if err := hydratePlaygroundGeneration(&current); err != nil {
				return err
			}
			unchanged := current.Status == result.Status && current.TaskId == result.TaskId &&
				current.Error == result.Error && slices.Equal(current.Outputs, result.Outputs)
			if !unchanged {
				return ErrPlaygroundGenerationFinalized
			}
			result = current
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func DeletePlaygroundGeneration(userId int, rawId string) error {
	if userId <= 0 {
		return errors.New("user id must be greater than zero")
	}
	id, err := normalizePlaygroundGenerationId(rawId)
	if err != nil {
		return err
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		var generation PlaygroundGeneration
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", id, userId).First(&generation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPlaygroundGenerationNotFound
			}
			return err
		}
		if err := tx.Where("generation_id = ? AND user_id = ?", id, userId).Delete(&PlaygroundGenerationAsset{}).Error; err != nil {
			return err
		}
		result := tx.Where("id = ? AND user_id = ?", id, userId).Delete(&PlaygroundGeneration{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrPlaygroundGenerationNotFound
		}
		return nil
	})
	return err
}

package model

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPlaygroundGenerationTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&User{}, &PlaygroundGeneration{}, &PlaygroundGenerationAsset{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&PlaygroundGenerationAsset{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&PlaygroundGeneration{}).Error)
	require.NoError(t, DB.Unscoped().Where("id IN ?", []int{101, 202}).Delete(&User{}).Error)
	for _, userId := range []int{101, 202} {
		require.NoError(t, DB.Create(&User{
			Id:       userId,
			Username: fmt.Sprintf("pg-user-%d", userId),
			Password: "playground-test-password",
			Role:     1,
			Status:   common.UserStatusEnabled,
			Group:    "default",
			AffCode:  fmt.Sprintf("pg-history-%020d", userId),
		}).Error)
	}
	t.Cleanup(func() {
		DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&PlaygroundGenerationAsset{})
		DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&PlaygroundGeneration{})
		DB.Unscoped().Where("id IN ?", []int{101, 202}).Delete(&User{})
	})
}

func playgroundGenerationInput(operation string) PlaygroundGenerationCreate {
	return PlaygroundGenerationCreate{
		Operation:       operation,
		Model:           "deepwl/test-media-model",
		Group:           "default",
		Prompt:          "A clean product photograph",
		Parameters:      map[string]interface{}{"resolution": "1024x1024"},
		ContractHash:    "contract-hash",
		ContractVersion: 2,
		PricingVersion:  "pricing-version",
		QuotedQuota:     1234,
		Amount:          0.02468,
	}
}

func TestPlaygroundGenerationLifecycleAndOwnership(t *testing.T) {
	setupPlaygroundGenerationTest(t)

	created, err := CreatePlaygroundGeneration(101, playgroundGenerationInput(PlaygroundGenerationOperationVideo))
	require.NoError(t, err)
	require.Len(t, created.Id, 32)
	assert.Equal(t, PlaygroundGenerationStatusPending, created.Status)
	assert.Empty(t, created.Outputs)
	assert.Zero(t, created.CompletedAt)

	otherUserItems, total, err := ListPlaygroundGenerations(202, PlaygroundGenerationOperationVideo, 0, 20)
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, otherUserItems)

	taskId := "provider-task-123"
	pendingStatus := PlaygroundGenerationStatusPending
	updated, err := UpdatePlaygroundGeneration(101, created.Id, PlaygroundGenerationUpdate{
		TaskId: &taskId,
		Status: &pendingStatus,
	})
	require.NoError(t, err)
	assert.Equal(t, taskId, updated.TaskId)
	assert.Zero(t, updated.CompletedAt)

	outputURLs := []string{"https://cdn.example.com/result.mp4?signature=abc"}
	succeededStatus := PlaygroundGenerationStatusSucceeded
	updated, err = UpdatePlaygroundGeneration(101, created.Id, PlaygroundGenerationUpdate{
		Outputs: &outputURLs,
		Status:  &succeededStatus,
	})
	require.NoError(t, err)
	assert.Equal(t, outputURLs, updated.Outputs)
	assert.Positive(t, updated.CompletedAt)

	_, err = UpdatePlaygroundGeneration(202, created.Id, PlaygroundGenerationUpdate{Status: &succeededStatus})
	assert.ErrorIs(t, err, ErrPlaygroundGenerationNotFound)
	assert.ErrorIs(t, DeletePlaygroundGeneration(202, created.Id), ErrPlaygroundGenerationNotFound)

	failedStatus := PlaygroundGenerationStatusFailed
	failureMessage := "late provider failure"
	_, err = UpdatePlaygroundGeneration(101, created.Id, PlaygroundGenerationUpdate{
		Status: &failedStatus,
		Error:  &failureMessage,
	})
	assert.ErrorIs(t, err, ErrPlaygroundGenerationFinalized)

	items, total, err := ListPlaygroundGenerations(101, PlaygroundGenerationOperationVideo, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, taskId, items[0].TaskId)
	assert.Equal(t, outputURLs, items[0].Outputs)
	assert.Equal(t, "1024x1024", items[0].Parameters["resolution"])

	require.NoError(t, DeletePlaygroundGeneration(101, created.Id))
	_, total, err = ListPlaygroundGenerations(101, PlaygroundGenerationOperationVideo, 0, 20)
	require.NoError(t, err)
	assert.Zero(t, total)
}

func TestPlaygroundGenerationValidatesPersistentPayload(t *testing.T) {
	setupPlaygroundGenerationTest(t)

	tests := []struct {
		name    string
		mutate  func(*PlaygroundGenerationCreate)
		message string
	}{
		{
			name: "data output URL",
			mutate: func(input *PlaygroundGenerationCreate) {
				input.Status = PlaygroundGenerationStatusSucceeded
				input.Outputs = []string{"data:image/png;base64,AAAA"}
			},
			message: "absolute HTTP(S) URL",
		},
		{
			name: "blob output URL",
			mutate: func(input *PlaygroundGenerationCreate) {
				input.Status = PlaygroundGenerationStatusSucceeded
				input.Outputs = []string{"blob:https://api.carlab.top/result"}
			},
			message: "absolute HTTP(S) URL",
		},
		{
			name: "relative output URL",
			mutate: func(input *PlaygroundGenerationCreate) {
				input.Status = PlaygroundGenerationStatusSucceeded
				input.Outputs = []string{"/result.png"}
			},
			message: "absolute HTTP(S) URL",
		},
		{
			name: "too many outputs",
			mutate: func(input *PlaygroundGenerationCreate) {
				input.Status = PlaygroundGenerationStatusSucceeded
				input.Outputs = make([]string, playgroundGenerationMaxOutputs+1)
				for index := range input.Outputs {
					input.Outputs[index] = "https://cdn.example.com/result.png"
				}
			},
			message: "16 entries or fewer",
		},
		{
			name: "prompt too long",
			mutate: func(input *PlaygroundGenerationCreate) {
				input.Prompt = strings.Repeat("p", playgroundGenerationMaxPromptLength+1)
			},
			message: "prompt is required",
		},
		{
			name: "too many parameters",
			mutate: func(input *PlaygroundGenerationCreate) {
				input.Parameters = make(map[string]interface{}, playgroundGenerationMaxParameters+1)
				for index := 0; index <= playgroundGenerationMaxParameters; index++ {
					input.Parameters[string(rune(index+1))] = index
				}
			},
			message: "128 entries or fewer",
		},
		{
			name: "encoded outputs exceed mysql text",
			mutate: func(input *PlaygroundGenerationCreate) {
				input.Status = PlaygroundGenerationStatusSucceeded
				input.Outputs = make([]string, playgroundGenerationMaxOutputs)
				for index := range input.Outputs {
					input.Outputs[index] = "https://example.com/?value=" + strings.Repeat("&", 2000)
				}
			},
			message: "outputs must be 65535 bytes or fewer",
		},
		{
			name: "invalid amount",
			mutate: func(input *PlaygroundGenerationCreate) {
				input.Amount = math.NaN()
			},
			message: "amount must be between",
		},
		{
			name: "failed without error",
			mutate: func(input *PlaygroundGenerationCreate) {
				input.Status = PlaygroundGenerationStatusFailed
			},
			message: "must contain an error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := playgroundGenerationInput(PlaygroundGenerationOperationImage)
			test.mutate(&input)
			_, err := CreatePlaygroundGeneration(101, input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.message)
		})
	}
}

func TestPlaygroundGenerationAllowsSuccessfulNonPersistentResult(t *testing.T) {
	setupPlaygroundGenerationTest(t)
	input := playgroundGenerationInput(PlaygroundGenerationOperationImage)
	input.Status = PlaygroundGenerationStatusSucceeded
	input.Outputs = []string{}

	generation, err := CreatePlaygroundGeneration(101, input)
	require.NoError(t, err)
	assert.Equal(t, PlaygroundGenerationStatusSucceeded, generation.Status)
	assert.Empty(t, generation.Outputs)
	assert.Positive(t, generation.CompletedAt)
}

func TestListPlaygroundGenerationsFiltersAndSortsNewestFirst(t *testing.T) {
	setupPlaygroundGenerationTest(t)

	oldImage, err := CreatePlaygroundGeneration(101, playgroundGenerationInput(PlaygroundGenerationOperationImage))
	require.NoError(t, err)
	video, err := CreatePlaygroundGeneration(101, playgroundGenerationInput(PlaygroundGenerationOperationVideo))
	require.NoError(t, err)
	newImage, err := CreatePlaygroundGeneration(101, playgroundGenerationInput(PlaygroundGenerationOperationImage))
	require.NoError(t, err)

	require.NoError(t, DB.Model(&PlaygroundGeneration{}).Where("id = ?", oldImage.Id).Update("created_at", 100).Error)
	require.NoError(t, DB.Model(&PlaygroundGeneration{}).Where("id = ?", video.Id).Update("created_at", 300).Error)
	require.NoError(t, DB.Model(&PlaygroundGeneration{}).Where("id = ?", newImage.Id).Update("created_at", 200).Error)

	items, total, err := ListPlaygroundGenerations(101, PlaygroundGenerationOperationImage, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 2)
	assert.Equal(t, []string{newImage.Id, oldImage.Id}, []string{items[0].Id, items[1].Id})

	items, total, err = ListPlaygroundGenerations(101, PlaygroundGenerationOperationImage, 1, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 1)
	assert.Equal(t, oldImage.Id, items[0].Id)
}

func TestPlaygroundGenerationEnforcesPerUserHistoryLimit(t *testing.T) {
	setupPlaygroundGenerationTest(t)
	records := make([]PlaygroundGeneration, playgroundGenerationMaximumPerUser)
	for index := range records {
		records[index] = PlaygroundGeneration{
			Id:             fmt.Sprintf("%032x", index+1),
			UserId:         101,
			Operation:      PlaygroundGenerationOperationImage,
			Model:          "deepwl/test-media-model",
			Group:          "default",
			Prompt:         "history limit fixture",
			ParametersJSON: `{}`,
			OutputsJSON:    `[]`,
			Status:         PlaygroundGenerationStatusPending,
			CreatedAt:      int64(index + 1),
			UpdatedAt:      int64(index + 1),
		}
	}
	require.NoError(t, DB.CreateInBatches(&records, 100).Error)

	_, err := CreatePlaygroundGeneration(101, playgroundGenerationInput(PlaygroundGenerationOperationImage))
	assert.ErrorIs(t, err, ErrPlaygroundGenerationLimit)

	_, err = CreatePlaygroundGeneration(202, playgroundGenerationInput(PlaygroundGenerationOperationImage))
	require.NoError(t, err)
}

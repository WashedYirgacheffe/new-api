package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type playgroundGenerationCreatePayload struct {
	Operation       string                 `json:"operation"`
	Model           string                 `json:"model"`
	Group           string                 `json:"group"`
	Prompt          string                 `json:"prompt"`
	Parameters      map[string]interface{} `json:"parameters"`
	ContractHash    string                 `json:"contract_hash"`
	ContractVersion int                    `json:"contract_version"`
	PricingVersion  string                 `json:"pricing_version"`
	QuotedQuota     int                    `json:"quoted_quota"`
	Amount          float64                `json:"amount"`
}

type playgroundGenerationUpdatePayload struct {
	Outputs *[]string `json:"outputs"`
	TaskId  *string   `json:"task_id"`
	Status  *string   `json:"status"`
	Error   *string   `json:"error"`
}

type playgroundGenerationAssetPayload struct {
	Ordinal int    `json:"ordinal"`
	DataURL string `json:"data_url"`
}

func CreatePlaygroundGeneration(c *gin.Context) {
	var payload playgroundGenerationCreatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, err)
		return
	}
	generation, err := model.CreatePlaygroundGeneration(c.GetInt("id"), model.PlaygroundGenerationCreate{
		Operation:       payload.Operation,
		Model:           payload.Model,
		Group:           payload.Group,
		Prompt:          payload.Prompt,
		Parameters:      payload.Parameters,
		Status:          model.PlaygroundGenerationStatusPending,
		ContractHash:    payload.ContractHash,
		ContractVersion: payload.ContractVersion,
		PricingVersion:  payload.PricingVersion,
		QuotedQuota:     payload.QuotedQuota,
		Amount:          payload.Amount,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, generation)
}

func ListPlaygroundGenerations(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListPlaygroundGenerations(
		c.GetInt("id"),
		c.Query("operation"),
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
	})
}

func UpdatePlaygroundGeneration(c *gin.Context) {
	var payload playgroundGenerationUpdatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, err)
		return
	}
	generation, err := model.UpdatePlaygroundGeneration(c.GetInt("id"), c.Param("id"), model.PlaygroundGenerationUpdate{
		Outputs: payload.Outputs,
		TaskId:  payload.TaskId,
		Status:  payload.Status,
		Error:   payload.Error,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, generation)
}

func DeletePlaygroundGeneration(c *gin.Context) {
	if err := service.DeletePlaygroundGenerationHistory(c.GetInt("id"), c.Param("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func UploadPlaygroundGenerationAsset(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.PlaygroundAssetRequestBytes)
	var payload playgroundGenerationAssetPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, err)
		return
	}
	asset, err := service.PersistPlaygroundGenerationAsset(
		c.GetInt("id"),
		c.Param("id"),
		payload.Ordinal,
		payload.DataURL,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, asset)
}

func GetPlaygroundGenerationAsset(c *gin.Context) {
	ordinal, err := strconv.Atoi(c.Param("ordinal"))
	if err != nil {
		common.ApiError(c, errors.New("asset ordinal must be an integer"))
		return
	}
	asset, path, err := service.ResolvePlaygroundGenerationAsset(c.GetInt("id"), c.Param("id"), ordinal)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", asset.MimeType)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(path)
}

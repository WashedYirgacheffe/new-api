package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type modelRouteSavePayload struct {
	CanonicalModel string                          `json:"canonical_model"`
	Operation      string                          `json:"operation"`
	Policy         string                          `json:"policy"`
	Enabled        *bool                           `json:"enabled"`
	Targets        []model.ModelRouteTargetPayload `json:"targets"`
	ExpectedHash   string                          `json:"expected_hash"`
}

func modelRouteExpectedHash(c *gin.Context, explicit string) string {
	expectedHash := strings.TrimSpace(explicit)
	if expectedHash == "" {
		expectedHash = strings.TrimSpace(c.GetHeader("If-Match"))
	}
	expectedHash = strings.TrimPrefix(expectedHash, "W/")
	return strings.Trim(expectedHash, `"`)
}

func writeModelRouteError(c *gin.Context, err error) {
	if errors.Is(err, model.ErrModelRouteConflict) {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	common.ApiError(c, err)
}

func GetModelRouteGroups(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	groups, total, err := model.ListModelRouteGroups(
		c.Query("model"),
		c.Query("operation"),
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":     groups,
		"total":     total,
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
	})
}

func GetModelRouteDetail(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" {
		common.ApiErrorMsg(c, "model is required")
		return
	}
	relations, err := model.GetModelRouteRelations(modelName, c.Query("operation"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, relations)
}

func SaveModelRouteGroup(c *gin.Context) {
	var payload modelRouteSavePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, err)
		return
	}
	group, err := model.SaveModelRouteGroup(model.ModelRouteGroupPayload{
		CanonicalModel: payload.CanonicalModel,
		Operation:      payload.Operation,
		Policy:         payload.Policy,
		Enabled:        payload.Enabled,
		Targets:        payload.Targets,
	}, modelRouteExpectedHash(c, payload.ExpectedHash))
	if err != nil {
		writeModelRouteError(c, err)
		return
	}
	common.ApiSuccess(c, group)
}

func DeleteModelRouteGroup(c *gin.Context) {
	canonicalModel := strings.TrimSpace(c.Query("model"))
	operation := strings.TrimSpace(c.Query("operation"))
	if canonicalModel == "" || operation == "" {
		common.ApiErrorMsg(c, "model and operation are required")
		return
	}
	if err := model.DeleteModelRouteGroup(
		canonicalModel,
		operation,
		modelRouteExpectedHash(c, c.Query("expected_hash")),
	); err != nil {
		writeModelRouteError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func setupPlaygroundContext(c *gin.Context, requestedGroup string) *types.NewAPIError {
	useAccessToken := c.GetBool("use_access_token")
	if useAccessToken {
		return types.NewError(errors.New("暂不支持使用 access token"), types.ErrorCodeAccessDenied, types.ErrOptionWithSkipRetry())
	}

	userId := c.GetInt("id")
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}
	userCache.WriteContext(c)

	group := strings.TrimSpace(requestedGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	if group == "" {
		group = userCache.Group
	}
	if group != userCache.Group && !service.GroupInUserUsableGroups(userCache.Group, group) {
		return types.NewError(fmt.Errorf("无权使用分组 %s", group), types.ErrorCodeAccessDenied, types.ErrOptionWithSkipRetry())
	}

	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("playground-%s", group),
		Group:  group,
	}
	if err := middleware.SetupContextForToken(c, tempToken); err != nil {
		return types.NewError(err, types.ErrorCodeAccessDenied, types.ErrOptionWithSkipRetry())
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
	return nil
}

func writePlaygroundError(c *gin.Context, err *types.NewAPIError) {
	if err == nil {
		return
	}
	c.JSON(err.StatusCode, gin.H{"error": err.ToOpenAIError()})
}

func GetPlaygroundModelCatalog(c *gin.Context) {
	if err := setupPlaygroundContext(c, c.Query("group")); err != nil {
		writePlaygroundError(c, err)
		return
	}
	GetTokenModelCatalog(c)
}

func QuotePlaygroundModel(c *gin.Context) {
	if err := setupPlaygroundContext(c, c.Query("group")); err != nil {
		writePlaygroundError(c, err)
		return
	}
	QuoteTokenModel(c)
}

func Playground(c *gin.Context) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		return
	}
	if newAPIError = setupPlaygroundContext(c, relayInfo.UsingGroup); newAPIError != nil {
		return
	}

	Relay(c, types.RelayFormatOpenAI)
}

func PlaygroundImage(c *gin.Context) {
	if err := setupPlaygroundContext(c, c.Query("group")); err != nil {
		writePlaygroundError(c, err)
		return
	}
	c.Set("relay_mode", relayconstant.RelayModeImagesGenerations)
	Relay(c, types.RelayFormatOpenAIImage)
}

func PlaygroundGemini(c *gin.Context) {
	if err := setupPlaygroundContext(c, c.Query("group")); err != nil {
		writePlaygroundError(c, err)
		return
	}
	Relay(c, types.RelayFormatGemini)
}

func PlaygroundVideo(c *gin.Context) {
	if err := setupPlaygroundContext(c, c.Query("group")); err != nil {
		writePlaygroundError(c, err)
		return
	}
	RelayTask(c)
}

func PlaygroundVideoFetch(c *gin.Context) {
	if err := setupPlaygroundContext(c, c.Query("group")); err != nil {
		writePlaygroundError(c, err)
		return
	}
	RelayTaskFetch(c)
}

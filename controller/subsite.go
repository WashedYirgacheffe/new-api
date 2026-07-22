package controller

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	subsiteCodePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)
	subsiteDomainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)
)

type subsiteClaimableItem struct {
	model.Subsite
	Claimed bool `json:"claimed"`
}

type subsiteClaimRequest struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

type subsiteMutationRequest struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	Domain        string `json:"domain"`
	Version       string `json:"version"`
	RouteGroup    string `json:"route_group"`
	ClaimPassword string `json:"claim_password"`
	Enabled       *bool  `json:"enabled"`
}

type subsiteModelsRequest struct {
	ModelIds []string `json:"model_ids"`
}

type subsiteCatalogModel struct {
	tokenCatalogModel
	Enabled bool `json:"enabled"`
}

type subsiteModelsPayload struct {
	Site            model.Subsite         `json:"site"`
	Items           []subsiteCatalogModel `json:"items"`
	EnabledModelIds []string              `json:"enabled_model_ids"`
}

type subsiteTokenCatalogPayload struct {
	Items               []tokenCatalogModel             `json:"items"`
	Profiles            []modelOperationProfileContract `json:"profiles"`
	Total               int                             `json:"total"`
	TokenGroup          string                          `json:"token_group"`
	RoutingGroups       []string                        `json:"routing_groups"`
	PricingVersion      string                          `json:"pricing_version"`
	SupportedModelTypes []string                        `json:"supported_model_types"`
	Subsite             model.Subsite                   `json:"subsite"`
}

func subsiteError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"message": message,
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func isSubsiteConflict(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "duplicate entry")
}

func normalizeSubsiteFields(request *subsiteMutationRequest) error {
	request.Code = strings.ToLower(strings.TrimSpace(request.Code))
	request.Name = strings.TrimSpace(request.Name)
	request.Domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(request.Domain)), ".")
	request.Version = strings.TrimSpace(request.Version)
	request.RouteGroup = strings.TrimSpace(request.RouteGroup)
	if !subsiteCodePattern.MatchString(request.Code) {
		return errors.New("code must be 2-64 lowercase letters, numbers, underscores, or hyphens")
	}
	if request.Name == "" || len(request.Name) > 128 {
		return errors.New("name is required and must be 128 characters or fewer")
	}
	if !subsiteDomainPattern.MatchString(request.Domain) || len(request.Domain) > 255 {
		return errors.New("domain is invalid")
	}
	if request.Version == "" || len(request.Version) > 32 {
		return errors.New("version is required and must be 32 characters or fewer")
	}
	if request.RouteGroup == "" || len(request.RouteGroup) > 64 {
		return errors.New("route_group is required and must be 64 characters or fewer")
	}
	return nil
}

func routeGroupCatalogPricing(routeGroup string) []model.Pricing {
	routableModels := getRoutableModelNames(modelListGroups{
		tokenGroup:  routeGroup,
		ownerGroups: []string{routeGroup},
	})
	routableSet := make(map[string]struct{}, len(routableModels))
	for _, modelName := range routableModels {
		routableSet[modelName] = struct{}{}
	}
	pricing := make([]model.Pricing, 0)
	for _, item := range model.GetPricing() {
		if model.IsRuntimeOnlySubsiteModel(item.ModelName) {
			continue
		}
		if _, ok := routableSet[item.ModelName]; !ok {
			continue
		}
		pricing = append(pricing, item)
	}
	sort.Slice(pricing, func(i, j int) bool {
		return pricing[i].ModelName < pricing[j].ModelName
	})
	return pricing
}

func requireSubsiteAccess(c *gin.Context, site *model.Subsite) bool {
	if c.GetInt("role") >= common.RoleAdminUser {
		return true
	}
	allowed, err := model.IsSubsiteAdmin(c.GetInt("id"), site.Id)
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return false
	}
	if !allowed {
		subsiteError(c, http.StatusForbidden, "subsite_access_denied", "this subsite has not been claimed by the current user")
		return false
	}
	return true
}

func buildSubsiteModelsPayload(site *model.Subsite) (subsiteModelsPayload, error) {
	pricing := routeGroupCatalogPricing(site.RouteGroup)
	catalog, err := buildTokenCatalogPayload(pricing, modelListGroups{
		tokenGroup:  site.RouteGroup,
		ownerGroups: []string{site.RouteGroup},
	})
	if err != nil {
		return subsiteModelsPayload{}, err
	}
	enabledModels, err := model.GetSubsiteModels(site.Id)
	if err != nil {
		return subsiteModelsPayload{}, err
	}
	enabledSet := make(map[string]struct{}, len(enabledModels))
	for _, item := range enabledModels {
		enabledSet[item.ModelName] = struct{}{}
	}
	items := make([]subsiteCatalogModel, 0, len(catalog.Items))
	enabledModelIds := make([]string, 0, len(enabledModels))
	for _, item := range catalog.Items {
		_, enabled := enabledSet[item.ModelId]
		items = append(items, subsiteCatalogModel{tokenCatalogModel: item, Enabled: enabled})
		if enabled {
			enabledModelIds = append(enabledModelIds, item.ModelId)
		}
	}
	return subsiteModelsPayload{
		Site:            *site,
		Items:           items,
		EnabledModelIds: enabledModelIds,
	}, nil
}

func GetClaimableSubsites(c *gin.Context) {
	sites, err := model.ListSubsites()
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	claims, err := model.GetSubsiteClaims(c.GetInt("id"))
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	items := make([]subsiteClaimableItem, 0, len(sites))
	for _, site := range sites {
		if !site.Enabled {
			continue
		}
		items = append(items, subsiteClaimableItem{Subsite: site, Claimed: claims[site.Id]})
	}
	common.ApiSuccess(c, gin.H{"items": items})
}

func ClaimSubsite(c *gin.Context) {
	var request subsiteClaimRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		subsiteError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	request.Code = strings.ToLower(strings.TrimSpace(request.Code))
	if request.Code == "" || request.Password == "" {
		subsiteError(c, http.StatusBadRequest, "invalid_request", "code and password are required")
		return
	}
	site, err := model.GetSubsiteByCode(request.Code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			subsiteError(c, http.StatusNotFound, "subsite_not_found", "subsite not found")
			return
		}
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	if !site.Enabled {
		subsiteError(c, http.StatusServiceUnavailable, "subsite_disabled", "subsite is disabled")
		return
	}
	if err := model.ClaimSubsite(site, c.GetInt("id"), request.Password); err != nil {
		if errors.Is(err, model.ErrSubsiteClaimPasswordInvalid) {
			subsiteError(c, http.StatusBadRequest, "invalid_claim_password", "claim password is invalid")
			return
		}
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	common.ApiSuccess(c, site)
}

func GetSubsiteModels(c *gin.Context) {
	site, err := model.GetSubsiteByCode(c.Param("code"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			subsiteError(c, http.StatusNotFound, "subsite_not_found", "subsite not found")
			return
		}
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	if !requireSubsiteAccess(c, site) {
		return
	}
	payload, err := buildSubsiteModelsPayload(site)
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	common.ApiSuccess(c, payload)
}

func UpdateSubsiteModels(c *gin.Context) {
	site, err := model.GetSubsiteByCode(c.Param("code"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			subsiteError(c, http.StatusNotFound, "subsite_not_found", "subsite not found")
			return
		}
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	if !requireSubsiteAccess(c, site) {
		return
	}
	var request subsiteModelsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		subsiteError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	allPricing := make(map[string]model.Pricing)
	for _, item := range model.GetPricing() {
		allPricing[item.ModelName] = item
	}
	routeCatalog, err := buildTokenCatalogPayload(routeGroupCatalogPricing(site.RouteGroup), modelListGroups{
		tokenGroup:  site.RouteGroup,
		ownerGroups: []string{site.RouteGroup},
	})
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	eligible := make(map[string]tokenCatalogModel, len(routeCatalog.Items))
	for _, item := range routeCatalog.Items {
		eligible[item.ModelId] = item
	}
	normalized := make([]string, 0, len(request.ModelIds))
	seen := make(map[string]struct{}, len(request.ModelIds))
	for _, value := range request.ModelIds {
		modelName := strings.TrimSpace(value)
		if modelName == "" {
			continue
		}
		if _, exists := seen[modelName]; exists {
			continue
		}
		seen[modelName] = struct{}{}
		if model.IsRuntimeOnlySubsiteModel(modelName) {
			subsiteError(c, http.StatusConflict, "model_runtime_only", fmt.Sprintf("model %s is a runtime-only variant and cannot be enabled directly", modelName))
			return
		}
		if _, exists := allPricing[modelName]; !exists {
			subsiteError(c, http.StatusNotFound, "model_not_found", fmt.Sprintf("model %s is unknown or disabled", modelName))
			return
		}
		catalogItem, exists := eligible[modelName]
		if !exists || !catalogItem.Routable {
			subsiteError(c, http.StatusServiceUnavailable, "model_not_routable", fmt.Sprintf("model %s is not routable in group %s", modelName, site.RouteGroup))
			return
		}
		if !relayhelper.HasModelBillingConfig(modelName) || !catalogItem.PriceReady {
			subsiteError(c, http.StatusConflict, "model_price_not_ready", fmt.Sprintf("model %s has no billing configuration", modelName))
			return
		}
		if !catalogItem.ProfileReady {
			subsiteError(c, http.StatusConflict, "model_contract_not_ready", fmt.Sprintf("model %s has no dispatch-ready profile binding", modelName))
			return
		}
		normalized = append(normalized, modelName)
	}
	if err := model.ReplaceSubsiteModels(site.Id, normalized); err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	payload, err := buildSubsiteModelsPayload(site)
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	common.ApiSuccess(c, payload)
}

func GetSubsites(c *gin.Context) {
	sites, err := model.ListSubsites()
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"items": sites})
}

func CreateSubsite(c *gin.Context) {
	var request subsiteMutationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		subsiteError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := normalizeSubsiteFields(&request); err != nil {
		subsiteError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.ClaimPassword == "" {
		subsiteError(c, http.StatusBadRequest, "invalid_request", "claim_password is required")
		return
	}
	hash, err := common.Password2Hash(request.ClaimPassword)
	if err != nil {
		subsiteError(c, http.StatusBadRequest, "invalid_claim_password", err.Error())
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	site := model.Subsite{
		Code:              request.Code,
		Name:              request.Name,
		Domain:            request.Domain,
		Version:           request.Version,
		RouteGroup:        request.RouteGroup,
		ClaimPasswordHash: hash,
		Enabled:           enabled,
	}
	if err := model.CreateSubsite(&site); err != nil {
		if isSubsiteConflict(err) {
			subsiteError(c, http.StatusConflict, "subsite_conflict", "subsite code or domain already exists")
			return
		}
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	common.ApiSuccess(c, site)
}

func UpdateSubsite(c *gin.Context) {
	site, err := model.GetSubsiteByCode(c.Param("code"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			subsiteError(c, http.StatusNotFound, "subsite_not_found", "subsite not found")
			return
		}
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	var request subsiteMutationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		subsiteError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(request.Code) == "" {
		request.Code = site.Code
	}
	if err := normalizeSubsiteFields(&request); err != nil {
		subsiteError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	site.Code = request.Code
	site.Name = request.Name
	site.Domain = request.Domain
	site.Version = request.Version
	site.RouteGroup = request.RouteGroup
	if request.Enabled != nil {
		site.Enabled = *request.Enabled
	}
	if request.ClaimPassword != "" {
		site.ClaimPasswordHash, err = common.Password2Hash(request.ClaimPassword)
		if err != nil {
			subsiteError(c, http.StatusBadRequest, "invalid_claim_password", err.Error())
			return
		}
	}
	if err := model.UpdateSubsite(site); err != nil {
		if isSubsiteConflict(err) {
			subsiteError(c, http.StatusConflict, "subsite_conflict", "subsite code or domain already exists")
			return
		}
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	updated, err := model.GetSubsiteByID(site.Id)
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	common.ApiSuccess(c, updated)
}

func DeleteSubsite(c *gin.Context) {
	site, err := model.GetSubsiteByCode(c.Param("code"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			subsiteError(c, http.StatusNotFound, "subsite_not_found", "subsite not found")
			return
		}
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	if err := model.DeleteSubsite(site.Id); err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	common.ApiSuccess(c, nil)
}

func GetSubsiteCatalogByDomain(c *gin.Context) {
	site, err := model.GetSubsiteByDomain(c.Param("domain"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			subsiteError(c, http.StatusNotFound, "subsite_not_found", "subsite not found")
			return
		}
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	if !site.Enabled {
		subsiteError(c, http.StatusServiceUnavailable, "subsite_disabled", "subsite is disabled")
		return
	}
	groups, err := getModelListGroups(c)
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	routeGroupMatched := groups.tokenGroup != "auto" &&
		len(groups.ownerGroups) == 1 &&
		groups.ownerGroups[0] == site.RouteGroup
	if !routeGroupMatched {
		subsiteError(c, http.StatusForbidden, "subsite_route_group_mismatch", fmt.Sprintf("token route group does not include subsite route group %s", site.RouteGroup))
		return
	}
	pricing, _, err := tokenScopedPricing(c)
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	enabledModels, err := model.GetSubsiteModels(site.Id)
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	enabledSet := make(map[string]struct{}, len(enabledModels))
	for _, item := range enabledModels {
		enabledSet[item.ModelName] = struct{}{}
	}
	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if _, enabled := enabledSet[item.ModelName]; enabled {
			filtered = append(filtered, item)
		}
	}
	catalog, err := buildTokenCatalogPayload(filtered, groups)
	if err != nil {
		subsiteError(c, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	common.ApiSuccess(c, subsiteTokenCatalogPayload{
		Items:               catalog.Items,
		Profiles:            catalog.Profiles,
		Total:               catalog.Total,
		TokenGroup:          catalog.TokenGroup,
		RoutingGroups:       catalog.RoutingGroups,
		PricingVersion:      catalog.PricingVersion,
		SupportedModelTypes: catalog.SupportedModelTypes,
		Subsite:             *site,
	})
}

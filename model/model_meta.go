package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	NameRuleExact = iota
	NameRulePrefix
	NameRuleContains
	NameRuleSuffix
)

type BoundChannel struct {
	Name            string `json:"name"`
	Type            int    `json:"type"`
	ChannelProvider string `json:"channel_provider,omitempty"`
}

type ModelChannelProvider struct {
	ModelID         int    `json:"model_id" gorm:"primaryKey;autoIncrement:false"`
	ChannelProvider string `json:"channel_provider" gorm:"type:varchar(64);primaryKey;autoIncrement:false;index"`
}

type Model struct {
	Id           int            `json:"id"`
	ModelName    string         `json:"model_name" gorm:"size:128;not null;uniqueIndex:uk_model_name_delete_at,priority:1"`
	DisplayName  string         `json:"display_name,omitempty" gorm:"type:varchar(128)"`
	ModelType    string         `json:"model_type,omitempty" gorm:"type:varchar(32);index"`
	Description  string         `json:"description,omitempty" gorm:"type:text"`
	SourceURL    string         `json:"source_url,omitempty" gorm:"type:text"`
	Icon         string         `json:"icon,omitempty" gorm:"type:varchar(128)"`
	Tags         string         `json:"tags,omitempty" gorm:"type:varchar(255)"`
	VendorID     int            `json:"vendor_id,omitempty" gorm:"index"`
	Endpoints    string         `json:"endpoints,omitempty" gorm:"type:text"`
	Status       int            `json:"status" gorm:"default:1"`
	SyncOfficial int            `json:"sync_official" gorm:"default:1"`
	CreatedTime  int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime  int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index;uniqueIndex:uk_model_name_delete_at,priority:2"`

	BoundChannels    []BoundChannel `json:"bound_channels,omitempty" gorm:"-"`
	ChannelProviders []string       `json:"channel_providers,omitempty" gorm:"-"`
	EnableGroups     []string       `json:"enable_groups,omitempty" gorm:"-"`
	QuotaTypes       []int          `json:"quota_types,omitempty" gorm:"-"`
	NameRule         int            `json:"name_rule" gorm:"default:0"`

	MatchedModels []string `json:"matched_models,omitempty" gorm:"-"`
	MatchedCount  int      `json:"matched_count,omitempty" gorm:"-"`
}

func (mi *Model) Insert() error {
	now := common.GetTimestamp()
	mi.CreatedTime = now
	mi.UpdatedTime = now

	// 保存原始值（因为 Create 后可能被 GORM 的 default 标签覆盖为 1）
	originalStatus := mi.Status
	originalSyncOfficial := mi.SyncOfficial

	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(mi).Error; err != nil {
			return err
		}
		if err := tx.Model(&Model{}).Where("id = ?", mi.Id).Updates(map[string]interface{}{
			"status":        originalStatus,
			"sync_official": originalSyncOfficial,
		}).Error; err != nil {
			return err
		}
		return replaceModelChannelProviders(tx, mi.Id, mi.ChannelProviders)
	})
}

func IsModelNameDuplicated(id int, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	var cnt int64
	err := DB.Model(&Model{}).Where("model_name = ? AND id <> ?", name, id).Count(&cnt).Error
	return cnt > 0, err
}

func (mi *Model) Update() error {
	mi.UpdatedTime = common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing Model
		if err := lockForUpdate(tx).Select("id").First(&existing, mi.Id).Error; err != nil {
			return err
		}
		if err := tx.Model(&Model{}).Where("id = ?", mi.Id).
			Select("model_name", "display_name", "model_type", "description", "source_url", "icon", "tags", "vendor_id", "endpoints", "status", "sync_official", "name_rule", "updated_time").
			Updates(mi).Error; err != nil {
			return err
		}
		return replaceModelChannelProviders(tx, mi.Id, mi.ChannelProviders)
	})
}

func (mi *Model) Delete() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("model_id = ?", mi.Id).Delete(&ModelChannelProvider{}).Error; err != nil {
			return err
		}
		return tx.Delete(mi).Error
	})
}

func replaceModelChannelProviders(tx *gorm.DB, modelID int, providers []string) error {
	if providers == nil {
		return nil
	}
	if err := tx.Where("model_id = ?", modelID).Delete(&ModelChannelProvider{}).Error; err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(providers))
	rows := make([]ModelChannelProvider, 0, len(providers))
	for _, provider := range providers {
		provider = strings.ToLower(strings.TrimSpace(provider))
		if provider == "" {
			continue
		}
		if len(provider) > 64 {
			return fmt.Errorf("API channel provider must be 64 characters or fewer")
		}
		if _, exists := seen[provider]; exists {
			continue
		}
		seen[provider] = struct{}{}
		rows = append(rows, ModelChannelProvider{ModelID: modelID, ChannelProvider: provider})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

func GetVendorModelCounts() (map[int64]int64, error) {
	var stats []struct {
		VendorID int64
		Count    int64
	}
	if err := DB.Model(&Model{}).
		Select("vendor_id as vendor_id, count(*) as count").
		Group("vendor_id").
		Scan(&stats).Error; err != nil {
		return nil, err
	}
	m := make(map[int64]int64, len(stats))
	for _, s := range stats {
		m[s.VendorID] = s.Count
	}
	return m, nil
}

func GetChannelProviderModelCounts() (map[string]int64, error) {
	type providerModelRow struct {
		ModelID         int
		ChannelProvider string
	}
	var catalogProviders []providerModelRow
	if err := DB.Table("model_channel_providers").
		Select("model_channel_providers.model_id, model_channel_providers.channel_provider").
		Joins("JOIN models ON models.id = model_channel_providers.model_id").
		Where("models.deleted_at IS NULL").
		Scan(&catalogProviders).Error; err != nil {
		return nil, err
	}
	var runtimeProviders []providerModelRow
	if err := DB.Table("models").
		Select("models.id as model_id, channels.channel_provider as channel_provider").
		Joins("JOIN abilities ON abilities.model = models.model_name").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("models.deleted_at IS NULL AND abilities.enabled = ? AND channels.status = ? AND channels.channel_provider <> ?", true, common.ChannelStatusEnabled, "").
		Scan(&runtimeProviders).Error; err != nil {
		return nil, err
	}
	providerModels := make(map[string]map[int]struct{})
	for _, item := range append(catalogProviders, runtimeProviders...) {
		models, exists := providerModels[item.ChannelProvider]
		if !exists {
			models = make(map[int]struct{})
			providerModels[item.ChannelProvider] = models
		}
		models[item.ModelID] = struct{}{}
	}
	counts := make(map[string]int64, len(providerModels))
	for provider, models := range providerModels {
		counts[provider] = int64(len(models))
	}
	return counts, nil
}

func GetChannelProvidersByModelsMap(modelIDs []int) (map[int][]string, error) {
	providerSets := make(map[int]map[string]struct{})
	if len(modelIDs) == 0 {
		return map[int][]string{}, nil
	}
	type providerModelRow struct {
		ModelID         int
		ChannelProvider string
	}
	var catalogProviders []providerModelRow
	if err := DB.Table("model_channel_providers").
		Select("model_id, channel_provider").
		Where("model_id IN ?", modelIDs).
		Scan(&catalogProviders).Error; err != nil {
		return nil, err
	}
	var runtimeProviders []providerModelRow
	if err := DB.Table("models").
		Select("models.id as model_id, channels.channel_provider as channel_provider").
		Joins("JOIN abilities ON abilities.model = models.model_name").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("models.id IN ? AND abilities.enabled = ? AND channels.status = ? AND channels.channel_provider <> ?", modelIDs, true, common.ChannelStatusEnabled, "").
		Scan(&runtimeProviders).Error; err != nil {
		return nil, err
	}
	for _, item := range append(catalogProviders, runtimeProviders...) {
		providers, exists := providerSets[item.ModelID]
		if !exists {
			providers = make(map[string]struct{})
			providerSets[item.ModelID] = providers
		}
		providers[item.ChannelProvider] = struct{}{}
	}
	result := make(map[int][]string, len(providerSets))
	for modelID, providers := range providerSets {
		for provider := range providers {
			result[modelID] = append(result[modelID], provider)
		}
		sort.Strings(result[modelID])
	}
	return result, nil
}

func GetAllModels(offset int, limit int) ([]*Model, error) {
	var models []*Model
	err := DB.Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error
	return models, err
}

func GetBoundChannelsByModelsMap(modelNames []string) (map[string][]BoundChannel, error) {
	result := make(map[string][]BoundChannel)
	if len(modelNames) == 0 {
		return result, nil
	}
	type row struct {
		Model           string
		Name            string
		Type            int
		ChannelProvider string
	}
	var rows []row
	err := DB.Table("channels").
		Select("abilities.model as model, channels.name as name, channels.type as type, channels.channel_provider as channel_provider").
		Joins("JOIN abilities ON abilities.channel_id = channels.id").
		Where("abilities.model IN ? AND abilities.enabled = ? AND channels.status = ?", modelNames, true, common.ChannelStatusEnabled).
		Distinct().
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.Model] = append(result[r.Model], BoundChannel{
			Name:            r.Name,
			Type:            r.Type,
			ChannelProvider: r.ChannelProvider,
		})
	}
	return result, nil
}

func normalizeLookupValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func GetPreferredModelOwnerChannelTypes(modelNames []string, groups []string) (map[string]int, error) {
	result := make(map[string]int)
	modelNames = normalizeLookupValues(modelNames)
	if len(modelNames) == 0 {
		return result, nil
	}

	type row struct {
		Model       string
		ChannelType int
	}
	var rows []row

	query := DB.Table("abilities").
		Select("abilities.model as model, channels.type as channel_type").
		Joins("JOIN channels ON abilities.channel_id = channels.id").
		Where("abilities.model IN ? AND abilities.enabled = ? AND channels.status = ?", modelNames, true, common.ChannelStatusEnabled).
		Order("COALESCE(abilities.priority, 0) DESC").
		Order("abilities.weight DESC").
		Order("abilities.channel_id ASC")

	groups = normalizeLookupValues(groups)
	if len(groups) > 0 {
		query = query.Where("abilities."+commonGroupCol+" IN ?", groups)
	}

	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, r := range rows {
		if _, ok := result[r.Model]; ok {
			continue
		}
		result[r.Model] = r.ChannelType
	}
	return result, nil
}

func SearchModels(keyword string, vendor string, channelProvider string, status string, syncOfficial string, offset int, limit int) ([]*Model, int64, error) {
	var models []*Model
	db := DB.Model(&Model{})
	channelProvider = strings.ToLower(strings.TrimSpace(channelProvider))
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("models.model_name LIKE ? OR models.display_name LIKE ? OR models.model_type LIKE ? OR models.description LIKE ? OR models.tags LIKE ?", like, like, like, like, like)
	}
	if vendor != "" {
		if vid, err := strconv.Atoi(vendor); err == nil {
			db = db.Where("models.vendor_id = ?", vid)
		} else {
			db = db.Joins("JOIN vendors ON vendors.id = models.vendor_id").Where("vendors.name LIKE ?", "%"+vendor+"%")
		}
	}
	if channelProvider != "" {
		catalogProviderQuery := DB.Table("model_channel_providers").
			Select("1").
			Where("model_channel_providers.model_id = models.id").
			Where("model_channel_providers.channel_provider = ?", channelProvider)
		runtimeProviderQuery := DB.Table("abilities").
			Select("1").
			Joins("JOIN channels ON channels.id = abilities.channel_id").
			Where("abilities.model = models.model_name").
			Where("abilities.enabled = ?", true).
			Where("channels.status = ?", common.ChannelStatusEnabled).
			Where("channels.channel_provider = ?", channelProvider)
		db = db.Where("EXISTS (?) OR EXISTS (?)", catalogProviderQuery, runtimeProviderQuery)
	}
	if value, err := strconv.Atoi(status); status != "" && err == nil {
		db = db.Where("models.status = ?", value)
	}
	if value, err := strconv.Atoi(syncOfficial); syncOfficial != "" && err == nil {
		db = db.Where("models.sync_official = ?", value)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := db.Order("models.id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	return models, total, nil
}

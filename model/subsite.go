package model

import (
	"errors"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DefaultSubsiteCode        = "superseed"
	DefaultSubsiteDomain      = "cjzz.top"
	DefaultSubsiteVersion     = "v0.9.0"
	DefaultSubsiteRouteGroup  = "gold"
	defaultSubsitePasswordEnv = "SUBSITE_DEFAULT_CLAIM_PASSWORD"
)

var ErrSubsiteClaimPasswordInvalid = errors.New("subsite claim password is invalid")

var defaultSubsiteModels = []string{
	"deepwl/gpt-image-2",
	"deepwl/gpt-image-2-c",
	"fox-gpt-image-2",
	"nodyhub/gpt-image-2",
	"deepwl/gemini-3-pro-image-preview-c",
	"deepwl/gemini-3.1-flash-image-preview-c",
	"deepwl/gemini-2.5-flash-image",
	"deepwl/omni-fast",
	"deepwl/veo_3_1",
	"deepwl/veo_3_1-fast",
	"deepwl/grok-1.5-video-6s",
	"deepwl/grok-video-3",
	"nodyhub/grok-video-3",
	"deepwl/gemini-3.5-flash",
	"nodyhub/gemini-3.5-flash",
	"deepwl/gpt-5.6-luna",
	"deepwl/gpt-5.6-terra",
}

var runtimeOnlySubsiteModels = map[string]struct{}{
	"deepwl/omni-fast-v2v":      {},
	"deepwl/grok-1.5-video-10s": {},
	"deepwl/grok-video-3-10s":   {},
	"deepwl/grok-video-3-15s":   {},
}

type Subsite struct {
	Id                int    `json:"id"`
	Code              string `json:"code" gorm:"type:varchar(64);not null;uniqueIndex"`
	Name              string `json:"name" gorm:"type:varchar(128);not null"`
	Domain            string `json:"domain" gorm:"type:varchar(255);not null;uniqueIndex"`
	Version           string `json:"version" gorm:"type:varchar(32);not null"`
	RouteGroup        string `json:"route_group" gorm:"type:varchar(64);not null"`
	ClaimPasswordHash string `json:"-" gorm:"type:varchar(255);not null"`
	Enabled           bool   `json:"enabled" gorm:"not null"`
	CreatedTime       int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime       int64  `json:"updated_time" gorm:"bigint"`
}

type SubsiteAdmin struct {
	Id          int   `json:"id"`
	SubsiteId   int   `json:"subsite_id" gorm:"not null;uniqueIndex:uk_subsite_admin,priority:1;index"`
	UserId      int   `json:"user_id" gorm:"not null;uniqueIndex:uk_subsite_admin,priority:2;index"`
	CreatedTime int64 `json:"created_time" gorm:"bigint"`
}

type SubsiteModel struct {
	Id          int    `json:"id"`
	SubsiteId   int    `json:"subsite_id" gorm:"not null;uniqueIndex:uk_subsite_model,priority:1;index"`
	ModelName   string `json:"model_name" gorm:"type:varchar(255);not null;uniqueIndex:uk_subsite_model,priority:2;index"`
	SortOrder   int    `json:"sort_order" gorm:"not null"`
	Enabled     bool   `json:"enabled" gorm:"not null"`
	CreatedTime int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime int64  `json:"updated_time" gorm:"bigint"`
}

func DefaultSubsiteModelNames() []string {
	return append([]string(nil), defaultSubsiteModels...)
}

func IsRuntimeOnlySubsiteModel(modelName string) bool {
	_, ok := runtimeOnlySubsiteModels[strings.TrimSpace(modelName)]
	return ok
}

func ListSubsites() ([]Subsite, error) {
	var sites []Subsite
	err := DB.Order("code ASC").Find(&sites).Error
	return sites, err
}

func GetSubsiteByID(id int) (*Subsite, error) {
	var site Subsite
	err := DB.First(&site, id).Error
	return &site, err
}

func GetSubsiteByCode(code string) (*Subsite, error) {
	var site Subsite
	err := DB.Where("code = ?", strings.ToLower(strings.TrimSpace(code))).First(&site).Error
	return &site, err
}

func GetSubsiteByDomain(domain string) (*Subsite, error) {
	var site Subsite
	err := DB.Where("domain = ?", strings.ToLower(strings.TrimSpace(domain))).First(&site).Error
	return &site, err
}

func CreateSubsite(site *Subsite) error {
	now := common.GetTimestamp()
	site.CreatedTime = now
	site.UpdatedTime = now
	return DB.Create(site).Error
}

func UpdateSubsite(site *Subsite) error {
	site.UpdatedTime = common.GetTimestamp()
	updates := map[string]interface{}{
		"code":         site.Code,
		"name":         site.Name,
		"domain":       site.Domain,
		"version":      site.Version,
		"route_group":  site.RouteGroup,
		"enabled":      site.Enabled,
		"updated_time": site.UpdatedTime,
	}
	if site.ClaimPasswordHash != "" {
		updates["claim_password_hash"] = site.ClaimPasswordHash
	}
	return DB.Model(&Subsite{}).Where("id = ?", site.Id).Updates(updates).Error
}

func DeleteSubsite(siteID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subsite_id = ?", siteID).Delete(&SubsiteAdmin{}).Error; err != nil {
			return err
		}
		if err := tx.Where("subsite_id = ?", siteID).Delete(&SubsiteModel{}).Error; err != nil {
			return err
		}
		return tx.Delete(&Subsite{}, siteID).Error
	})
}

func ClaimSubsite(site *Subsite, userID int, password string) error {
	if !common.ValidatePasswordAndHash(password, site.ClaimPasswordHash) {
		return ErrSubsiteClaimPasswordInvalid
	}
	claim := SubsiteAdmin{
		SubsiteId:   site.Id,
		UserId:      userID,
		CreatedTime: common.GetTimestamp(),
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&claim).Error
}

func GetSubsiteClaims(userID int) (map[int]bool, error) {
	var claims []SubsiteAdmin
	if err := DB.Where("user_id = ?", userID).Find(&claims).Error; err != nil {
		return nil, err
	}
	result := make(map[int]bool, len(claims))
	for _, claim := range claims {
		result[claim.SubsiteId] = true
	}
	return result, nil
}

func IsSubsiteAdmin(userID int, subsiteID int) (bool, error) {
	var count int64
	err := DB.Model(&SubsiteAdmin{}).Where("user_id = ? AND subsite_id = ?", userID, subsiteID).Count(&count).Error
	return count > 0, err
}

func GetSubsiteModels(subsiteID int) ([]SubsiteModel, error) {
	var models []SubsiteModel
	err := DB.Where("subsite_id = ? AND enabled = ?", subsiteID, true).Order("sort_order ASC, model_name ASC").Find(&models).Error
	return models, err
}

func ReplaceSubsiteModels(subsiteID int, modelNames []string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var site Subsite
		if err := lockForUpdate(tx).Select("id").First(&site, subsiteID).Error; err != nil {
			return err
		}
		if err := tx.Where("subsite_id = ?", subsiteID).Delete(&SubsiteModel{}).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		rows := make([]SubsiteModel, 0, len(modelNames))
		seen := make(map[string]struct{}, len(modelNames))
		for index, value := range modelNames {
			modelName := strings.TrimSpace(value)
			if modelName == "" {
				continue
			}
			if _, ok := seen[modelName]; ok {
				continue
			}
			seen[modelName] = struct{}{}
			rows = append(rows, SubsiteModel{
				SubsiteId:   subsiteID,
				ModelName:   modelName,
				SortOrder:   index,
				Enabled:     true,
				CreatedTime: now,
				UpdatedTime: now,
			})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

func SeedDefaultSubsite() error {
	var existing Subsite
	err := DB.Where("code = ? OR domain = ?", DefaultSubsiteCode, DefaultSubsiteDomain).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	claimPassword := strings.TrimSpace(os.Getenv(defaultSubsitePasswordEnv))
	if claimPassword == "" {
		claimPassword, err = common.GenerateRandomKey(48)
		if err != nil {
			return err
		}
	}
	hash, err := common.Password2Hash(claimPassword)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		now := common.GetTimestamp()
		site := Subsite{
			Code:              DefaultSubsiteCode,
			Name:              "超级种子",
			Domain:            DefaultSubsiteDomain,
			Version:           DefaultSubsiteVersion,
			RouteGroup:        DefaultSubsiteRouteGroup,
			ClaimPasswordHash: hash,
			Enabled:           true,
			CreatedTime:       now,
			UpdatedTime:       now,
		}
		if err := tx.Create(&site).Error; err != nil {
			return err
		}
		rows := make([]SubsiteModel, 0, len(defaultSubsiteModels))
		for index, modelName := range defaultSubsiteModels {
			rows = append(rows, SubsiteModel{
				SubsiteId:   site.Id,
				ModelName:   modelName,
				SortOrder:   index,
				Enabled:     true,
				CreatedTime: now,
				UpdatedTime: now,
			})
		}
		return tx.Create(&rows).Error
	})
}

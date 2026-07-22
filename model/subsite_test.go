package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSubsiteModelTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&User{}, &Subsite{}, &SubsiteAdmin{}, &SubsiteModel{}))
}

func TestSeedDefaultSubsiteCreatesCanonicalModelsAndHashesPassword(t *testing.T) {
	setupSubsiteModelTest(t)
	claimPassword := "test-default-subsite-claim-password"
	t.Setenv(defaultSubsitePasswordEnv, claimPassword)
	if existing, err := GetSubsiteByCode(DefaultSubsiteCode); err == nil {
		require.NoError(t, DeleteSubsite(existing.Id))
	} else {
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}
	t.Cleanup(func() {
		if existing, err := GetSubsiteByCode(DefaultSubsiteCode); err == nil {
			_ = DeleteSubsite(existing.Id)
		}
	})

	require.NoError(t, SeedDefaultSubsite())
	require.NoError(t, SeedDefaultSubsite())

	site, err := GetSubsiteByCode(DefaultSubsiteCode)
	require.NoError(t, err)
	assert.Equal(t, DefaultSubsiteDomain, site.Domain)
	assert.Equal(t, DefaultSubsiteVersion, site.Version)
	assert.Equal(t, DefaultSubsiteRouteGroup, site.RouteGroup)
	assert.NotEqual(t, claimPassword, site.ClaimPasswordHash)
	assert.True(t, common.ValidatePasswordAndHash(claimPassword, site.ClaimPasswordHash))

	models, err := GetSubsiteModels(site.Id)
	require.NoError(t, err)
	require.Len(t, models, 17)
	for _, item := range models {
		assert.False(t, IsRuntimeOnlySubsiteModel(item.ModelName))
	}

	encoded, err := common.Marshal(site)
	require.NoError(t, err)
	assert.NotContains(t, strings.ToLower(string(encoded)), "password")
}

func TestSeedDefaultSubsiteUsesDefaultClaimPasswordWithoutEnvironmentOverride(t *testing.T) {
	setupSubsiteModelTest(t)
	t.Setenv(defaultSubsitePasswordEnv, "")
	if existing, err := GetSubsiteByCode(DefaultSubsiteCode); err == nil {
		require.NoError(t, DeleteSubsite(existing.Id))
	} else {
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}
	t.Cleanup(func() {
		if existing, err := GetSubsiteByCode(DefaultSubsiteCode); err == nil {
			_ = DeleteSubsite(existing.Id)
		}
	})

	require.NoError(t, SeedDefaultSubsite())
	site, err := GetSubsiteByCode(DefaultSubsiteCode)
	require.NoError(t, err)
	assert.True(t, common.ValidatePasswordAndHash(DefaultSubsiteClaimPassword, site.ClaimPasswordHash))
}

func TestSeedDefaultSubsiteIsIdempotentWhenCanonicalDomainAlreadyExists(t *testing.T) {
	setupSubsiteModelTest(t)
	DB.Where("code = ? OR domain = ?", DefaultSubsiteCode, DefaultSubsiteDomain).Delete(&Subsite{})
	hash, err := common.Password2Hash("renamed-site-password")
	require.NoError(t, err)
	site := &Subsite{
		Code:              "renamed-superseed",
		Name:              "Renamed Superseed",
		Domain:            DefaultSubsiteDomain,
		Version:           DefaultSubsiteVersion,
		RouteGroup:        DefaultSubsiteRouteGroup,
		ClaimPasswordHash: hash,
		Enabled:           true,
	}
	require.NoError(t, CreateSubsite(site))
	t.Cleanup(func() { _ = DeleteSubsite(site.Id) })

	require.NoError(t, SeedDefaultSubsite())

	var count int64
	require.NoError(t, DB.Model(&Subsite{}).
		Where("code = ? OR domain = ?", DefaultSubsiteCode, DefaultSubsiteDomain).
		Count(&count).Error)
	assert.EqualValues(t, 1, count)
	_, err = GetSubsiteByCode(DefaultSubsiteCode)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	stored, err := GetSubsiteByDomain(DefaultSubsiteDomain)
	require.NoError(t, err)
	assert.Equal(t, site.Code, stored.Code)
}

func TestClaimAndDeleteSubsiteDoNotChangeUserRole(t *testing.T) {
	setupSubsiteModelTest(t)
	hash, err := common.Password2Hash("claim-secret")
	require.NoError(t, err)
	site := &Subsite{
		Code:              "role-contract-site",
		Name:              "Role Contract Site",
		Domain:            "role-contract.example.com",
		Version:           "v1",
		RouteGroup:        "gold",
		ClaimPasswordHash: hash,
		Enabled:           true,
	}
	DB.Where("code = ?", site.Code).Delete(&Subsite{})
	require.NoError(t, CreateSubsite(site))

	user := &User{
		Username: "subsite-role-contract-user",
		Password: "hashed-for-test",
		Role:     common.RoleSubsiteAdminUser,
		Status:   common.UserStatusEnabled,
		AffCode:  "subsite-role-contract-aff",
	}
	DB.Where("username = ?", user.Username).Delete(&User{})
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() {
		DB.Where("username = ?", user.Username).Delete(&User{})
		if current, getErr := GetSubsiteByCode(site.Code); getErr == nil {
			_ = DeleteSubsite(current.Id)
		}
	})

	require.ErrorIs(t, ClaimSubsite(site, user.Id, "wrong"), ErrSubsiteClaimPasswordInvalid)
	require.NoError(t, ClaimSubsite(site, user.Id, "claim-secret"))
	require.NoError(t, DeleteSubsite(site.Id))

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	assert.Equal(t, common.RoleSubsiteAdminUser, stored.Role)
}

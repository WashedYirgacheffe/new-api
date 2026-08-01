package sora_test

import (
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/sora"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelContractPrepareAndBuildRequestBodyWritesStringSeconds(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:omni_outbound_contract?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	require.NoError(t, db.AutoMigrate(
		&model.ModelOperationProfile{},
		&model.ModelOperationProfileVersion{},
		&model.ModelOperationBinding{},
	))

	profile := model.ModelOperationProfile{
		ProfileKey:  "video.generate.omni-outbound-test",
		DisplayName: "Omni outbound test",
	}
	require.NoError(t, db.Create(&profile).Error)
	version := model.ModelOperationProfileVersion{
		ProfileId:        profile.Id,
		Version:          1,
		Operation:        "video.generate",
		EndpointType:     "openai-video",
		ExecutionMode:    "async",
		InputSchema:      `{"type":"object","properties":{"prompt":{"type":"string","minLength":1},"seconds":{"type":"integer","enum":[8,10],"default":10},"aspect_ratio":{"type":"string","enum":["16:9"],"default":"16:9"},"resolution":{"type":"string","enum":["720p"],"default":"720p"}},"required":["prompt"],"additionalProperties":false}`,
		UISchema:         `{}`,
		MaterialSchema:   `{}`,
		ResponseContract: "openai-video-task-v1",
		SmokeTest:        `{}`,
		Status:           model.ModelOperationProfileStatusPublished,
	}
	require.NoError(t, db.Create(&version).Error)

	const requestContract = `"request_contract":{"adapter":"openai-video","field_map":{"seconds":"seconds","aspect_ratio":"aspect_ratio","resolution":"resolution"},"coercions":{"seconds":"string"}},"parameter_defaults":{"seconds":10,"aspect_ratio":"16:9","resolution":"720p"}`
	testCases := []struct {
		name            string
		modelName       string
		overrides       string
		expectedSeconds float64
	}{
		{
			name:            "contract default",
			modelName:       "deepwl/omni-default-test",
			overrides:       `{` + requestContract + `}`,
			expectedSeconds: 10,
		},
		{
			name:            "contract forced override",
			modelName:       "deepwl/omni-forced-test",
			overrides:       `{` + requestContract + `,"parameter_overrides":{"seconds":8}}`,
			expectedSeconds: 8,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require.NoError(t, db.Create(&model.ModelOperationBinding{
				ModelName:       testCase.modelName,
				Operation:       version.Operation,
				ProfileKey:      profile.ProfileKey,
				ProfileVersion:  version.Version,
				ContractVersion: 1,
				Overrides:       testCase.overrides,
				Enabled:         true,
			}).Error)

			bodyText := `{"model":"` + testCase.modelName + `","prompt":"move"}`
			gin.SetMode(gin.TestMode)
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(bodyText))
			context.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(context) })
			request := &relaycommon.TaskSubmitReq{}
			require.NoError(t, common.Unmarshal([]byte(bodyText), request))
			info := &relaycommon.RelayInfo{
				OriginModelName: testCase.modelName,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "omni-fast-upstream",
				},
			}

			prepared, err := helper.PrepareModelOperationContractRequest(context, info, "video.generate", request)
			require.NoError(t, err)
			require.NotNil(t, prepared)
			expectedSeconds := strconv.Itoa(int(testCase.expectedSeconds))
			assert.Equal(t, testCase.expectedSeconds, prepared.EffectiveParameters["seconds"])
			assert.Equal(t, int(testCase.expectedSeconds), request.Duration)
			assert.Equal(t, expectedSeconds, request.Seconds)

			outbound, err := (&sora.TaskAdaptor{}).BuildRequestBody(context, info)
			require.NoError(t, err)
			outboundJSON, err := io.ReadAll(outbound)
			require.NoError(t, err)
			decoded := map[string]interface{}{}
			require.NoError(t, common.Unmarshal(outboundJSON, &decoded))
			assert.Equal(t, "omni-fast-upstream", decoded["model"])
			assert.Equal(t, expectedSeconds, decoded["seconds"])
			assert.NotContains(t, decoded, "duration")
		})
	}
}

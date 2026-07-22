package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRelayReAPITaskFetchScopesAndRedactsTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open("file:relay_reapi_fetch?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB := model.DB
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&model.Task{
		TaskID:     "task_public_re",
		UserId:     101,
		Platform:   constant.TaskPlatform("59"),
		Status:     model.TaskStatusInProgress,
		CreatedAt:  456,
		Properties: model.Properties{OriginModelName: "re/gpt-image-2"},
		Data:       []byte(`{"id":"upstream_secret_456","model":"gpt-image-2","status":"processing","created_at":456,"output":{"url":"https://cdn.example/output.png","task_id":"upstream_secret_456"}}`),
	}).Error)
	require.NoError(t, db.Create(&model.Task{
		TaskID:   "task_not_re",
		UserId:   101,
		Platform: constant.TaskPlatform("31"),
		Status:   model.TaskStatusInProgress,
		Data:     []byte(`{"id":"other"}`),
	}).Error)

	request := httptest.NewRequest(http.MethodGet, "/v1/re/tasks/task_public_re", nil)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Params = gin.Params{{Key: "task_id", Value: "task_public_re"}}
	context.Set("id", 101)
	RelayReAPITaskFetch(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"id":"task_public_re"`)
	assert.NotContains(t, recorder.Body.String(), "upstream_secret_456")

	foreignRequest := httptest.NewRequest(http.MethodGet, "/v1/re/tasks/task_public_re", nil)
	foreignRecorder := httptest.NewRecorder()
	foreignContext, _ := gin.CreateTestContext(foreignRecorder)
	foreignContext.Request = foreignRequest
	foreignContext.Params = gin.Params{{Key: "task_id", Value: "task_public_re"}}
	foreignContext.Set("id", 202)
	RelayReAPITaskFetch(foreignContext)
	assert.Equal(t, http.StatusNotFound, foreignRecorder.Code)

	nonRERequest := httptest.NewRequest(http.MethodGet, "/v1/re/tasks/task_not_re", nil)
	nonRERecorder := httptest.NewRecorder()
	nonREContext, _ := gin.CreateTestContext(nonRERecorder)
	nonREContext.Request = nonRERequest
	nonREContext.Params = gin.Params{{Key: "task_id", Value: "task_not_re"}}
	nonREContext.Set("id", 101)
	RelayReAPITaskFetch(nonREContext)
	assert.Equal(t, http.StatusNotFound, nonRERecorder.Code)
	assert.False(t, strings.Contains(nonRERecorder.Body.String(), "other"))
}

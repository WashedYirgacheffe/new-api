package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteModelOperationBindingErrorUsesConflictStatusForStaleContract(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeModelOperationBindingError(ctx, fmt.Errorf("%w: stale hash", model.ErrModelOperationBindingConflict))

	require.Equal(t, http.StatusConflict, recorder.Code)
	var response map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, false, response["success"])
	assert.Contains(t, response["message"], "stale hash")
}

func TestWriteModelOperationEvidenceErrorUsesConflictStatusForOwnershipChange(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeModelOperationEvidenceError(ctx, fmt.Errorf("%w: cannot change", model.ErrModelOperationParameterEvidenceConflict))

	require.Equal(t, http.StatusConflict, recorder.Code)
	var response map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, false, response["success"])
	assert.Contains(t, response["message"], "cannot change")
}

package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kiranshivaraju/loghunter/internal/api/response"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSON(t *testing.T) {
	w := httptest.NewRecorder()
	response.JSON(w, map[string]string{"name": "test"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	data := body["data"].(map[string]any)
	assert.Equal(t, "test", data["name"])
}

func TestCreated(t *testing.T) {
	w := httptest.NewRecorder()
	response.Created(w, map[string]string{"id": "abc"})

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	data := body["data"].(map[string]any)
	assert.Equal(t, "abc", data["id"])
}

func TestAccepted(t *testing.T) {
	w := httptest.NewRecorder()
	response.Accepted(w, map[string]string{"job_id": "j1"})

	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	data := body["data"].(map[string]any)
	assert.Equal(t, "j1", data["job_id"])
}

func TestCollection(t *testing.T) {
	w := httptest.NewRecorder()
	items := []map[string]string{{"id": "1"}, {"id": "2"}}
	meta := response.PaginationMeta{Page: 1, Limit: 20, Total: 50, HasNext: true}

	response.Collection(w, items, meta)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	data := body["data"].([]any)
	assert.Len(t, data, 2)

	m := body["meta"].(map[string]any)
	assert.Equal(t, float64(1), m["page"])
	assert.Equal(t, float64(20), m["limit"])
	assert.Equal(t, float64(50), m["total"])
	assert.Equal(t, true, m["has_next"])
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	response.Error(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid params", map[string][]string{
		"service": {"service is required"},
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	errObj := body["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
	assert.Equal(t, "Invalid params", errObj["message"])
	assert.NotNil(t, errObj["details"])
}

func TestError_NoDetails(t *testing.T) {
	w := httptest.NewRecorder()
	response.Error(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Not found", nil)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	errObj := body["error"].(map[string]any)
	assert.Equal(t, "RESOURCE_NOT_FOUND", errObj["code"])
	_, hasDetails := errObj["details"]
	assert.False(t, hasDetails)
}

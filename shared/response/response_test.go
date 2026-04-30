package response

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func runHandler(fn gin.HandlerFunc) (*httptest.ResponseRecorder, *gin.Context) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	fn(c)
	return w, c
}

func decode(t *testing.T, w *httptest.ResponseRecorder) Response {
	t.Helper()
	var r Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
	return r
}

func TestOK_StatusAndShape(t *testing.T) {
	w, _ := runHandler(func(c *gin.Context) { OK(c, gin.H{"id": 1}) })
	assert.Equal(t, 200, w.Code)
	body := decode(t, w)
	assert.True(t, body.Success)
	assert.Empty(t, body.Message)
	assert.NotNil(t, body.Data)
}

func TestCreated_201(t *testing.T) {
	w, _ := runHandler(func(c *gin.Context) { Created(c, "x") })
	assert.Equal(t, 201, w.Code)
	assert.True(t, decode(t, w).Success)
}

func TestError_PassesCodeAndMessage(t *testing.T) {
	w, _ := runHandler(func(c *gin.Context) { Error(c, 422, "bad input") })
	assert.Equal(t, 422, w.Code)
	body := decode(t, w)
	assert.False(t, body.Success)
	assert.Equal(t, "bad input", body.Message)
}

func TestPage_TotalPagesCalcRoundsUp(t *testing.T) {
	// total=23, size=10 → totalPages should be 3 (ceil division).
	w, _ := runHandler(func(c *gin.Context) {
		Page(c, []int{1, 2, 3}, 23, 1, 10)
	})
	assert.Equal(t, 200, w.Code)
	var raw struct {
		Success bool `json:"success"`
		Data    struct {
			TotalElements int64 `json:"totalElements"`
			TotalPages    int64 `json:"totalPages"`
			Page          int   `json:"page"`
			Size          int   `json:"size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.True(t, raw.Success)
	assert.EqualValues(t, 23, raw.Data.TotalElements)
	assert.EqualValues(t, 3, raw.Data.TotalPages, "23/10 should round up to 3 pages")
	assert.Equal(t, 1, raw.Data.Page)
	assert.Equal(t, 10, raw.Data.Size)
}

func TestPage_ExactMultipleNoExtraPage(t *testing.T) {
	w, _ := runHandler(func(c *gin.Context) {
		Page(c, []int{}, 20, 1, 10)
	})
	var raw struct {
		Data struct {
			TotalPages int64 `json:"totalPages"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.EqualValues(t, 2, raw.Data.TotalPages, "20/10 = exactly 2 pages, not 3")
}

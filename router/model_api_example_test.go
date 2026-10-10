package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelAPIExampleWriteRequiresAdministrator(t *testing.T) {
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	previousRateLimit := common.GlobalApiRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	t.Cleanup(func() { common.GlobalApiRateLimitEnable = previousRateLimit })

	for _, test := range []struct {
		name     string
		signedIn bool
		status   int
	}{
		{"anonymous", false, http.StatusUnauthorized},
		{"ordinary user", true, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(sessions.Sessions("test", cookie.NewStore([]byte("test-session-secret-32-bytes-long"))))
			engine.Use(func(c *gin.Context) {
				if test.signedIn {
					session := sessions.Default(c)
					session.Set("username", "ordinary-user")
					session.Set("role", common.RoleCommonUser)
					session.Set("id", 1)
					session.Set("status", common.UserStatusEnabled)
				}
				c.Next()
			})
			SetApiRouter(engine)
			body, err := common.Marshal(map[string]string{
				"model_name": "dsv41-flash", "endpoint_type": "openai", "language": "curl", "code": "curl example",
			})
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPut, "/api/models/api_example", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("New-Api-User", "1")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			assert.Equal(t, test.status, response.Code)
			var result struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.False(t, result.Success)
		})
	}
}

package middleware

import (
	"net/http"
	"one-api/common"
	"one-api/model"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GoogleOAuthAuth 中间件：路径校准和 OAuth token 验证
func GoogleOAuthAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// 1. 路径校准：检查路径是否包含 "batch" 关键词
		// 如果不包含 "batch"，说明使用了不支持的 URL
		// 注意：/google/token 接口不需要检查 batch 关键词
		if !strings.HasPrefix(path, "/google/token") {
			if !strings.Contains(strings.ToLower(path), "batch") {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "不支持的方法或路径",
				})
				c.Abort()
				return
			}
		}

		// 2. 对于非 /google/token 接口，验证 OAuth token
		if path != "/google/token" && !strings.HasPrefix(path, "/google/token") {
			// 获取 Authorization header
			authHeader := c.Request.Header.Get("Authorization")
			if authHeader == "" {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "未提供 Authorization header",
				})
				c.Abort()
				return
			}

			// 提取 Bearer token
			if !strings.HasPrefix(authHeader, "Bearer ") {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "Authorization header 格式错误，应为 'Bearer <token>'",
				})
				c.Abort()
				return
			}

			clientToken := strings.TrimPrefix(authHeader, "Bearer ")
			clientToken = strings.TrimSpace(clientToken)

			if clientToken == "" {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "Authorization token 为空",
				})
				c.Abort()
				return
			}

			// 查询 OAuth 表，根据 client_access_token 查找对应的记录
			oauth, err := model.GetOAuthByClientAccessToken(clientToken)
			if err != nil {
				if err == gorm.ErrRecordNotFound {
					c.JSON(http.StatusUnauthorized, gin.H{
						"error": "无效的 client_access_token",
					})
					c.Abort()
					return
				}
				common.LogError(c.Request.Context(), "查询 OAuth 表失败: "+err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "服务器内部错误",
				})
				c.Abort()
				return
			}

			if oauth == nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "无效的 client_access_token",
				})
				c.Abort()
				return
			}

			// 检查状态是否可用
			if oauth.Status != common.OAuthStatusEnabled {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "OAuth 配置已被禁用",
				})
				c.Abort()
				return
			}

			// 验证通过，将 oauth 信息存储到 context 中，供后续使用
			c.Set("oauth", oauth)
		}

		c.Next()
	}
}

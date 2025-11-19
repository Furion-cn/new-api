package controller

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"one-api/common"
	"one-api/model"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"gorm.io/gorm"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type GoogleTokenRequest struct {
	Type                    string `json:"type"`
	ProjectId               string `json:"project_id"`
	PrivateKeyId            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientId                string `json:"client_id"`
	AuthUri                 string `json:"auth_uri"`
	TokenUri                string `json:"token_uri"`
	AuthProviderX509CertUrl string `json:"auth_provider_x509_cert_url"`
	ClientX509CertUrl       string `json:"client_x509_cert_url"`
	UniverseDomain          string `json:"universe_domain"`
}

// ParseRSAPublicKey 从 PEM 格式字符串解析 RSA 公钥
func ParseRSAPublicKey(publicKeyPEM string) (*rsa.PublicKey, error) {
	// 清理 PEM 格式
	publicKeyPEM = strings.ReplaceAll(publicKeyPEM, "-----BEGIN PUBLIC KEY-----", "")
	publicKeyPEM = strings.ReplaceAll(publicKeyPEM, "-----END PUBLIC KEY-----", "")
	publicKeyPEM = strings.ReplaceAll(publicKeyPEM, "\r", "")
	publicKeyPEM = strings.ReplaceAll(publicKeyPEM, "\n", "")
	publicKeyPEM = strings.ReplaceAll(publicKeyPEM, "\\n", "")

	block, _ := pem.Decode([]byte("-----BEGIN PUBLIC KEY-----\n" + publicKeyPEM + "\n-----END PUBLIC KEY-----"))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the public key")
	}

	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaPublicKey, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}

	return rsaPublicKey, nil
}

// ExtractPublicKeyFromPrivateKey 从私钥中提取公钥
func ExtractPublicKeyFromPrivateKey(privateKeyPEM string) (string, error) {
	// 清理 PEM 格式
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "-----BEGIN RSA PRIVATE KEY-----", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "-----END RSA PRIVATE KEY-----", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "-----BEGIN PRIVATE KEY-----", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "-----END PRIVATE KEY-----", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "\r", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "\n", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "\\n", "")

	// 尝试 PKCS1 格式
	block, _ := pem.Decode([]byte("-----BEGIN RSA PRIVATE KEY-----\n" + privateKeyPEM + "\n-----END RSA PRIVATE KEY-----"))
	if block != nil {
		privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err == nil {
			// 从私钥提取公钥
			publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
			if err != nil {
				return "", err
			}
			publicKeyBlock := &pem.Block{
				Type:  "PUBLIC KEY",
				Bytes: publicKeyBytes,
			}
			return string(pem.EncodeToMemory(publicKeyBlock)), nil
		}
	}

	// 尝试 PKCS8 格式
	block, _ = pem.Decode([]byte("-----BEGIN PRIVATE KEY-----\n" + privateKeyPEM + "\n-----END PRIVATE KEY-----"))
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM block containing the private key")
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}

	rsaPrivateKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("not an RSA private key")
	}

	// 从私钥提取公钥
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&rsaPrivateKey.PublicKey)
	if err != nil {
		return "", err
	}
	publicKeyBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	return string(pem.EncodeToMemory(publicKeyBlock)), nil
}

// VerifyJWTToken 验证 JWT token 并返回 claims
func VerifyJWTToken(tokenString, publicKeyPEM string) (jwt.MapClaims, error) {
	// 解析公钥
	publicKey, err := ParseRSAPublicKey(publicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	// 解析并验证 token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// 验证签名算法
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token is not valid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("failed to parse claims")
	}

	return claims, nil
}

func GoogleToken(c *gin.Context) {
	// 解析 application/x-www-form-urlencoded 格式的请求体
	if err := c.Request.ParseForm(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "无法解析请求参数: " + err.Error(),
		})
		return
	}

	// 获取 assertion (JWT token)
	assertion := c.PostForm("assertion")
	if assertion == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "缺少 assertion 参数",
		})
		return
	}

	// 获取 grant_type（URL 解码）
	grantType, err := url.QueryUnescape(c.PostForm("grant_type"))
	if err != nil {
		grantType = c.PostForm("grant_type") // 如果解码失败，使用原始值
	}
	if grantType != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "unsupported_grant_type",
			"error_description": "不支持的 grant_type: " + grantType,
		})
		return
	}

	// 解析 JWT token（不验证签名，先提取信息）
	token, _, err := new(jwt.Parser).ParseUnverified(assertion, jwt.MapClaims{})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "无效的 JWT token: " + err.Error(),
		})
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "无法解析 JWT claims",
		})
		return
	}

	// 从 JWT header 中获取 kid (key id)
	kid, ok := token.Header["kid"].(string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "JWT token 中缺少 kid",
		})
		return
	}

	// 从 claims 中获取 iss (issuer/email)
	iss, ok := claims["iss"].(string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "JWT token 中缺少 iss",
		})
		return
	}

	// 根据 private_key_id (kid) 查询 OAuth 配置
	oauth, err := model.GetOAuthByPrivateKeyId(kid)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_grant",
				"error_description": "未找到匹配的 OAuth 配置",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":             "server_error",
			"error_description": "查询失败: " + err.Error(),
		})
		return
	}

	// 验证 client_email 是否匹配
	if oauth.ClientEmail != iss {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "issuer 不匹配",
		})
		return
	}

	if oauth.PublicKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "OAuth 配置中缺少公钥，且无法从 channel 配置中提取",
		})
		return
	}

	verifiedClaims, err := VerifyJWTToken(assertion, oauth.PublicKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "JWT token 验证失败: " + err.Error(),
		})
		return
	}

	// 检查 token 是否过期
	var exp int64
	switch v := verifiedClaims["exp"].(type) {
	case float64:
		exp = int64(v)
	case int64:
		exp = v
	case int:
		exp = int64(v)
	}
	if exp > 0 && time.Now().Unix() > exp {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "JWT token 已过期",
		})
		return
	}

	common.LogInfo(c, "token: "+fmt.Sprintf("%+v", token))
	common.LogInfo(c, "oauth: "+fmt.Sprintf("%+v", oauth))

	// 获取 access_token
	var accessToken string
	var expiresIn int

	// 如果配置了 channel_id，从 channel 获取 access_token
	if oauth.ChannelId > 0 {
		scope := "https://www.googleapis.com/auth/cloud-platform"
		if scopeStr, ok := verifiedClaims["scope"].(string); ok && scopeStr != "" {
			scope = scopeStr
		}

		accessToken, err = GetAccessToken(oauth.ChannelId, scope)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_grant",
				"error_description": "获取 access_token 失败: " + err.Error(),
			})
			return
		}
		// Google OAuth2 access_token 通常有效期为 3600 秒（1小时）
		expiresIn = 3600

		// 更新 OAuth model 对象和数据库
		err = model.SetOAuthAccessToken(oauth, accessToken, expiresIn)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":             "server_error",
				"error_description": "设置 access_token 失败: " + err.Error(),
			})
			return
		}
	} else {
		// 如果没有配置 channel_id，使用 OAuth 表中存储的 access_token
		accessToken = oauth.AccessToken
		if accessToken == "" {
			accessToken = assertion // 临时使用 assertion 作为 access_token
		}
		expiresIn = oauth.ExpiresIn
		if expiresIn == 0 {
			expiresIn = 3599 // 默认 3599 秒（约 1 小时）
		}
	}

	// 检查并生成 client_access_token（自签发的 token）
	clientAccessToken := oauth.ClientAccessToken
	if clientAccessToken == "" {
		// 生成新的 client access token
		clientAccessToken, err = common.GenerateKey()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":             "server_error",
				"error_description": "生成 client_access_token 失败: " + err.Error(),
			})
			return
		}

		// 保存到数据库
		err = model.SetOAuthClientAccessToken(oauth, clientAccessToken)
		if err != nil {
			common.LogError(c, fmt.Sprintf("保存 client_access_token 到数据库失败: %v", err))
			// 不返回错误，继续返回 token
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": clientAccessToken,
		"expires_in":   expiresIn,
		"token_type":   "Bearer",
	})
}

func GetAccessToken(channelID int, scope string) (string, error) {
	if channelID <= 0 {
		return "", fmt.Errorf("channel_id 无效或未配置")
	}

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return "", fmt.Errorf("查询 channel 失败: %w", err)
	}

	if channel == nil {
		return "", fmt.Errorf("channel #%d 不存在", channelID)
	}

	if channel.Key == "" {
		return "", fmt.Errorf("channel #%d 的 Key 字段为空", channelID)
	}

	// 需要指定 scope（这里用最通用的 cloud-platform）
	conf, err := google.JWTConfigFromJSON([]byte(channel.Key), scope)
	if err != nil {
		return "", fmt.Errorf("解析 service account JSON 失败: %w", err)
	}

	// 创建带代理的 HTTP 客户端
	proxyClient := createProxyHTTPClient()

	// 创建带代理的 context
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, proxyClient)

	// 获取 tokenSource（使用带代理的 context）
	ts := conf.TokenSource(ctx)
	// 获取当前 Access Token
	accessToken, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("获取 access token 失败: %w", err)
	}

	return accessToken.AccessToken, nil
}

package controller

import (
	"net/http"
	"one-api/common"
	"one-api/model"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetUserBatchJobs 查询当前登录用户的所有 token 对应的 oauth 的所有 job_info 记录
func GetUserBatchJobs(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户未登录",
		})
		return
	}

	// 1. 查询当前用户的所有 token
	var tokens []*model.Token
	err := model.DB.Where("user_id = ?", userId).Find(&tokens).Error
	if err != nil {
		common.LogError(c.Request.Context(), "Failed to get user tokens: "+err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "查询用户 token 失败: " + err.Error(),
		})
		return
	}

	if len(tokens) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    []*model.BatchJobInfo{},
		})
		return
	}

	// 2. 提取所有 token ID
	tokenIds := make([]int, 0, len(tokens))
	for _, token := range tokens {
		tokenIds = append(tokenIds, token.Id)
	}

	// 3. 查询这些 token 对应的 oauth 记录
	var oauths []*model.OAuth
	err = model.DB.Where("token_id IN ?", tokenIds).Find(&oauths).Error
	if err != nil {
		common.LogError(c.Request.Context(), "Failed to get oauth records: "+err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "查询 OAuth 记录失败: " + err.Error(),
		})
		return
	}

	if len(oauths) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    []*model.BatchJobInfo{},
		})
		return
	}

	// 4. 提取所有 oauth ID
	oauthIds := make([]uint, 0, len(oauths))
	for _, oauth := range oauths {
		oauthIds = append(oauthIds, uint(oauth.ID))
	}

	// 5. 查询这些 oauth 对应的所有 BatchJobInfo
	var batchJobs []*model.BatchJobInfo
	err = model.DB.Where("oauth_id IN ?", oauthIds).Order("created_at DESC").Find(&batchJobs).Error
	if err != nil {
		// 检查是否是列不存在的错误
		if strings.Contains(err.Error(), "Unknown column 'oauth_id'") {
			common.LogError(c.Request.Context(), "oauth_id column does not exist in batch_job_info table. Please run database migration.")
			// 如果列不存在，返回空结果而不是错误
			batchJobs = []*model.BatchJobInfo{}
		} else {
			common.LogError(c.Request.Context(), "Failed to get batch job info: "+err.Error())
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "查询 BatchJobInfo 失败: " + err.Error(),
			})
			return
		}
	}

	if batchJobs == nil {
		batchJobs = []*model.BatchJobInfo{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    batchJobs,
	})
}

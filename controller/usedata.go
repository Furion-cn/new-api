package controller

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"one-api/model"
	"strconv"
	"time"
)

func GetAllQuotaDates(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	token_name := c.Query("token_name")
	dates, err := model.GetAllQuotaDates(startTimestamp, endTimestamp, username, token_name)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetBilling(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	dates, err := model.GetAllQuotaDates(startTimestamp, endTimestamp, username, "")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetUserQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 判断时间跨度是否超过 1 个月
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	dates, err := model.GetQuotaDataByUserId(userId, startTimestamp, endTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func ExportBillingExcel(c *gin.Context) {
	// 从查询参数获取时间范围
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("user_name")
	tokenname := c.Query("token_name")
	// 判断时间跨度是否超过 1 个月
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	if tokenname != "" && username == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "令牌名称和用户名称需要同时填写",
		})
	}
	// 转换时间戳为时间格式
	startTime := time.Unix(startTimestamp, 0)
	if startTime.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的开始时间格式",
		})
		return
	}

	endTime := time.Unix(endTimestamp, 0)
	if endTime.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的结束时间格式",
		})
		return
	}

	// 获取Excel数据
	excelBytes, err := model.GetBillingAndExportExcel(startTime.Unix(), endTime.Unix(), username, tokenname)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	// 设置文件名
	filename := fmt.Sprintf("billing_%s_%s.xlsx",
		startTime.Format("20060102"),
		endTime.Format("20060102"))
	if username != "" {
		filename = fmt.Sprintf("%s_billing_%s_%s.xlsx",
			username,
			startTime.Format("20060102"),
			endTime.Format("20060102"))
		if tokenname != "" {
			filename = fmt.Sprintf("%s_%s_billing_%s_%s.xlsx",
				username,
				tokenname,
				startTime.Format("20060102"),
				endTime.Format("20060102"))
		}
	}

	// 设置响应头
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Expires", "0")
	c.Header("Cache-Control", "must-revalidate")
	c.Header("Pragma", "public")

	// 写入响应
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", excelBytes)
}

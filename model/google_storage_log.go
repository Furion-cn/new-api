package model

import (
	"one-api/common"

	"gorm.io/gorm"
)

// GoogleStorageLog Google Storage 接口请求日志
type GoogleStorageLog struct {
	gorm.Model
	OAuthId  int    `json:"oauth_id" gorm:"index"`                   // 关联的 OAuth ID
	Method   string `json:"method" gorm:"type:varchar(10);index"`    // HTTP 方法
	URL      string `json:"url" gorm:"type:text"`                    // 请求 URL
	ClientIP string `json:"client_ip" gorm:"type:varchar(50);index"` // 客户端 IP
	common.RequestInfo
}

func (GoogleStorageLog) TableName() string {
	return "google_storage_logs"
}

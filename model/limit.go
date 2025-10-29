package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"one-api/common"
	"time"
)

type Limit struct {
	Id        int64  `json:"id" gorm:"primaryKey;autoIncrement;type:bigint"`
	UserId    int    `json:"user_id" gorm:"index;not null;type:int"`
	Username  string `json:"username" gorm:"index;type:varchar(64);not null"`
	ModelName string `json:"model_name" gorm:"index;type:varchar(100);not null"`
	TokenID   int    `json:"token_id" gorm:"index;not null;type:int"`
	TokenName string `json:"token_name" gorm:"index;type:varchar(100);not null"`

	RPMLimit            int   `json:"rpm_limit" gorm:"default:0;not null;type:int"`
	RPMLimitEnabled     bool  `json:"rpm_limit_enabled" gorm:"default:false;not null;type:tinyint(1)"`
	WaitDurationSeconds int64 `json:"wait_duration_seconds" gorm:"default:0;not null;type:bigint"`
	RelayTimeoutSeconds int64 `json:"relay_timeout_seconds" gorm:"default:0;not null;type:bigint"`
	CreatedAt           int64 `json:"created_at" gorm:"autoCreateTime;index;type:bigint"`
	UpdatedAt           int64 `json:"updated_at" gorm:"autoUpdateTime;type:bigint"`
}

// TableName 指定表名
func (Limit) TableName() string {
	return "limits"
}

// GetUserTokenModelLimit 根据用户ID、token名和模型名获取限流配置
func GetUserTokenModelLimit(userId int, tokenName, modelName string) (*Limit, error) {
	if userId == 0 || tokenName == "" || modelName == "" {
		return nil, errors.New("参数不能为空")
	}

	var limit Limit
	err := DB.Where("user_id = ? AND token_name = ? AND model_name = ?",
		userId, tokenName, modelName).First(&limit).Error

	if err != nil {
		return nil, err
	}

	return &limit, nil
}

// GetUserTokenModelLimitWithCache 带缓存的获取限流配置（1秒过期）
func GetUserTokenModelLimitWithCache(userId int, tokenName, modelName string) (*Limit, error) {
	// 构建缓存key
	cacheKey := fmt.Sprintf("limit_cache:%d:%s:%s", userId, tokenName, modelName)

	// 如果Redis未启用，直接查询数据库
	if !common.RedisEnabled {
		return GetUserTokenModelLimit(userId, tokenName, modelName)
	}

	// 尝试从Redis缓存获取
	ctx := context.Background()
	rdb := common.RDB

	cachedData, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil && cachedData != "" {
		// 缓存命中，解析JSON数据
		var limit Limit
		if err := json.Unmarshal([]byte(cachedData), &limit); err == nil {
			return &limit, nil
		}
		// JSON解析失败，继续查询数据库
	}

	// 缓存未命中或解析失败，查询数据库
	limit, err := GetUserTokenModelLimit(userId, tokenName, modelName)
	if err != nil {
		return nil, err
	}

	// 将结果存入缓存（1秒过期）
	if limit != nil {
		if limitJson, err := json.Marshal(limit); err == nil {
			rdb.Set(ctx, cacheKey, limitJson, time.Second)
		}
	}

	return limit, nil
}

package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// JobState Batch Prediction Job 状态
type JobState string

const (
	JobStatePending   JobState = "JOB_STATE_PENDING"
	JobStateRunning   JobState = "JOB_STATE_RUNNING"
	JobStateSucceeded JobState = "JOB_STATE_SUCCEEDED"
	JobStateFailed    JobState = "JOB_STATE_FAILED"
	JobStateCancelled JobState = "JOB_STATE_CANCELLED"
)

// BatchJobSource Batch Job 源配置
type BatchJobSource struct {
	Format      string   `json:"format"`       // jsonl, bigquery 等
	GcsUri      []string `json:"gcs_uri"`      // GCS URI 列表
	BigqueryUri *string  `json:"bigquery_uri"` // BigQuery URI，可选
}

// Value 实现 driver.Valuer 接口，用于数据库存储
func (b BatchJobSource) Value() (driver.Value, error) {
	return json.Marshal(b)
}

// Scan 实现 sql.Scanner 接口，用于数据库读取
func (b *BatchJobSource) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, b)
}

// BatchJobDestination Batch Job 目标配置
type BatchJobDestination struct {
	Format      string  `json:"format"`       // jsonl, bigquery 等
	GcsUri      *string `json:"gcs_uri"`      // GCS URI，可选
	BigqueryUri *string `json:"bigquery_uri"` // BigQuery URI，可选
}

// Value 实现 driver.Valuer 接口，用于数据库存储
func (b BatchJobDestination) Value() (driver.Value, error) {
	return json.Marshal(b)
}

// Scan 实现 sql.Scanner 接口，用于数据库读取
func (b *BatchJobDestination) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, b)
}

// BatchJobInfo Batch Prediction Job 信息
type BatchJobInfo struct {
	gorm.Model
	Name        string              `json:"name" gorm:"type:varchar(512);uniqueIndex;not null"`         // 格式: projects/{project_id}/locations/{location}/batchPredictionJobs/{job_id}
	DisplayName string              `json:"display_name" gorm:"type:varchar(255);uniqueIndex;not null"` // 显示名称
	State       JobState            `json:"state" gorm:"type:varchar(64)"`                              // Job 状态
	Error       *string             `json:"error" gorm:"type:text"`                                     // 错误信息，可选
	CreateTime  time.Time           `json:"create_time" gorm:"type:datetime"`                           // 创建时间
	StartTime   *time.Time          `json:"start_time" gorm:"type:datetime"`                            // 开始时间，可选
	EndTime     *time.Time          `json:"end_time" gorm:"type:datetime"`                              // 结束时间，可选
	UpdateTime  time.Time           `json:"update_time" gorm:"type:datetime"`                           // 更新时间
	ModelPath   string              `json:"model" gorm:"type:varchar(512);column:model"`                // 模型路径，如 'publishers/google/models/gemini-2.5-pro'
	Src         BatchJobSource      `json:"src" gorm:"type:json"`                                       // 源配置（JSON 序列化）
	Dest        BatchJobDestination `json:"dest" gorm:"type:json"`                                      // 目标配置（JSON 序列化）
	OAuthId     uint                `json:"oauth_id" gorm:"type:int;index;column:oauth_id"`             // 关联 OAuth 表的 ID
}

// TableName 指定表名（如果需要存储到数据库）
func (BatchJobInfo) TableName() string {
	return "batch_job_info"
}

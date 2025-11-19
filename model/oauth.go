package model

import (
	"errors"

	"gorm.io/gorm"
)

type OAuth struct {
	gorm.Model
	Type                    string `json:"type" gorm:"type:varchar(64);not null;default:'service_account'"`
	ProjectId               string `json:"project_id" gorm:"type:varchar(255);index"`
	PrivateKeyId            string `json:"private_key_id" gorm:"type:varchar(255);uniqueIndex"`
	PrivateKey              string `json:"private_key" gorm:"type:text;not null"`
	PublicKey               string `json:"public_key" gorm:"type:text"`
	ClientEmail             string `json:"client_email" gorm:"type:varchar(255);index"`
	ClientId                string `json:"client_id" gorm:"type:varchar(255)"`
	AuthUri                 string `json:"auth_uri" gorm:"type:varchar(512)"`
	TokenUri                string `json:"token_uri" gorm:"type:varchar(512)"`
	AuthProviderX509CertUrl string `json:"auth_provider_x509_cert_url" gorm:"type:varchar(512);column:auth_provider_x509_cert_url"`
	ClientX509CertUrl       string `json:"client_x509_cert_url" gorm:"type:varchar(512);column:client_x509_cert_url"`
	UniverseDomain          string `json:"universe_domain" gorm:"type:varchar(255);default:'googleapis.com'"`
	Name                    string `json:"name" gorm:"type:varchar(255);index"`
	Status                  int    `json:"status" gorm:"type:int;default:1"`
	AccessToken             string `json:"access_token" gorm:"type:text"`
	ClientAccessToken       string `json:"client_access_token" gorm:"type:text"`
	ExpiresIn               int    `json:"expires_in" gorm:"type:int;default:3599"`
	ChannelId               int    `json:"channel_id" gorm:"type:int;index"`
	TokenId                 int    `json:"token_id" gorm:"type:int;index"`
}

// TableName 指定表名
func (OAuth) TableName() string {
	return "oauth"
}

// GetOAuthByPrivateKeyId 根据 private_key_id 查询
func GetOAuthByPrivateKeyId(privateKeyId string) (*OAuth, error) {
	if privateKeyId == "" {
		return nil, nil
	}
	var oauth OAuth
	err := DB.First(&oauth, "private_key_id = ?", privateKeyId).Error
	if err != nil {
		return nil, err
	}
	return &oauth, nil
}

// GetOAuthByPrivateKeyIdAndPrivateKey 根据 private_key_id 和 private_key 查询
func GetOAuthByPrivateKeyIdAndPrivateKey(privateKeyId, privateKey string) (*OAuth, error) {
	if privateKeyId == "" || privateKey == "" {
		return nil, nil
	}
	var oauth OAuth
	err := DB.First(&oauth, "private_key_id = ? AND private_key = ?", privateKeyId, privateKey).Error
	if err != nil {
		return nil, err
	}
	return &oauth, nil
}

// SetOAuthAccessToken 更新 OAuth 的 access_token 和过期时间，同时更新 model 对象
func SetOAuthAccessToken(oauth *OAuth, accessToken string, expiresIn int) error {
	if oauth == nil {
		return errors.New("oauth 对象不能为空")
	}
	if oauth.ID == 0 {
		return errors.New("oauth_id 无效或未配置")
	}

	// 更新 model 对象的字段
	oauth.AccessToken = accessToken
	oauth.ExpiresIn = expiresIn

	// 更新数据库
	return DB.Model(oauth).Updates(map[string]interface{}{
		"access_token": accessToken,
		"expires_in":   expiresIn,
	}).Error
}

// SetOAuthClientAccessToken 更新 OAuth 的 client_access_token，同时更新 model 对象
func SetOAuthClientAccessToken(oauth *OAuth, clientAccessToken string) error {
	if oauth == nil {
		return errors.New("oauth 对象不能为空")
	}
	if oauth.ID == 0 {
		return errors.New("oauth_id 无效或未配置")
	}

	// 更新 model 对象的字段
	oauth.ClientAccessToken = clientAccessToken

	// 更新数据库
	return DB.Model(oauth).Update("client_access_token", clientAccessToken).Error
}

// GetOAuthByClientAccessToken 根据 client_access_token 查询 OAuth 记录（排除软删除）
func GetOAuthByClientAccessToken(clientAccessToken string) (*OAuth, error) {
	if clientAccessToken == "" {
		return nil, nil
	}
	var oauth OAuth
	// GORM 默认会排除软删除的记录（DeletedAt IS NULL）
	err := DB.Where("client_access_token = ?", clientAccessToken).First(&oauth).Error
	if err != nil {
		return nil, err
	}
	return &oauth, nil
}

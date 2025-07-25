package model

import (
	"fmt"
	"one-api/common"
	"one-api/setting/operation_setting"
	"sort"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id        int    `json:"id"`
	UserID    int    `json:"user_id" gorm:"index"`
	Username  string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	TokenName string `json:"token_name" gorm:"size:256;default:''"`
	ModelName string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	TokenUsed int    `json:"token_used" gorm:"default:0"`
	Count     int    `json:"count" gorm:"default:0"`
	Quota     int    `json:"quota" gorm:"default:0"`
}

// QuotaDataByDay 按天聚合的数据结构
type QuotaDataByDay struct {
	Username  string `json:"username"`
	TokenName string `json:"token_name"`
	ModelName string `json:"model_name"`
	Count     int    `json:"count"`
	Price     int    `json:"price"`
	TokenUsed int    `json:"token_used"`
	DateStr   string `json:"date_str"` // FROM_UNIXTIME返回的日期字符串
}

type BillingData struct {
	ChannelId         int    `json:"chanel_id"`
	ChannelName       string `json:"channel_name"`
	ChannelTag        string `json:"channel_tag"`
	Count             int    `json:"count"`
	ModelName         string `json:"model_name"`
	PromptTokens      int    `json:"prompt_tokens"`
	CompletionsTokens int    `json:"completions_tokens"`
}

type BillingJsonData struct {
	ChannelId          int     `json:"chanel_id"`
	CurrentDate        string  `json:"current_date"`
	ChannelName        string  `json:"channel_name"`
	ChannelTag         string  `json:"channel_tag"`
	Count              int     `json:"count"`
	ModelName          string  `json:"model_name"`
	PromptTokens       float32 `json:"prompt_tokens"`
	CompletionsTokens  float32 `json:"completions_tokens"`
	PromptPricing      float32 `json:"prompt_pricing"`
	CompletionsPricing float32 `json:"completions_pricing"`
	/**/ Cost float32 `json:"cost"`
}

func UpdateQuotaData() {
	// recover
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("UpdateQuotaData panic: %s", r))
		}
	}()
	for {
		if common.DataExportEnabled {
			common.SysLog("正在更新数据看板数据...")
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

func logQuotaDataCache(userId int, tokenName, username string, modelName string, quota int, createdAt int64, tokenUsed int) {
	key := fmt.Sprintf("%d-%s-%s-%s-%d", userId, username, tokenName, modelName, createdAt)
	quotaData, ok := CacheQuotaData[key]
	if ok {
		quotaData.Count += 1
		quotaData.Quota += quota
		quotaData.TokenUsed += tokenUsed
	} else {
		quotaData = &QuotaData{
			UserID:    userId,
			Username:  username,
			TokenName: tokenName,
			ModelName: modelName,
			CreatedAt: createdAt,
			Count:     1,
			Quota:     quota,
			TokenUsed: tokenUsed,
		}
	}
	CacheQuotaData[key] = quotaData
}

func LogQuotaData(userId int, tokenName, username string, modelName string, quota int, createdAt int64, tokenUsed int) {
	// 只精确到小时
	createdAt = createdAt - (createdAt % 3600)

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(userId, tokenName, username, modelName, quota, createdAt, tokenUsed)
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	size := len(CacheQuotaData)
	// 如果缓存中有数据，就保存到数据库中
	// 1. 先查询数据库中是否有数据
	// 2. 如果有数据，就更新数据
	// 3. 如果没有数据，就插入数据
	for _, quotaData := range CacheQuotaData {
		quotaDataDB := &QuotaData{}
		DB.Table("quota_data").Where("user_id = ? and token_name = ? and username = ? and model_name = ? and created_at = ?",
			quotaData.UserID, quotaData.TokenName, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt).First(quotaDataDB)
		if quotaDataDB.Id > 0 {
			//quotaDataDB.Count += quotaData.Count
			//quotaDataDB.Quota += quotaData.Quota
			//DB.Table("quota_data").Save(quotaDataDB)
			increaseQuotaData(quotaData.UserID, quotaData.TokenName, quotaData.Username, quotaData.ModelName, quotaData.Count, quotaData.Quota, quotaData.CreatedAt, quotaData.TokenUsed)
		} else {
			DB.Table("quota_data").Create(quotaData)
		}
	}
	CacheQuotaData = make(map[string]*QuotaData)
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据", size))
}

func increaseQuotaData(userId int, tokenname, username string, modelName string, count int, quota int, createdAt int64, tokenUsed int) {
	err := DB.Table("quota_data").Where("user_id = ? and token_name = ? and username = ? and model_name = ? and created_at = ?",
		userId, tokenname, username, modelName, createdAt).Updates(map[string]interface{}{
		"count":      gorm.Expr("count + ?", count),
		"quota":      gorm.Expr("quota + ?", quota),
		"token_used": gorm.Expr("token_used + ?", tokenUsed),
	}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("increaseQuotaData error: %s", err))
	}
}

func GetQuotaDataByUsername(username, tokenName string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	if tokenName != "" {
		err = DB.Table("quota_data").Where("username = ? and token_name = ? and created_at >= ? and created_at <= ?", username, tokenName, startTime, endTime).Find(&quotaDatas).Error
		return quotaDatas, err
	} else {
		err = DB.Table("quota_data").Where("username = ? and created_at >= ? and created_at <= ?", username, startTime, endTime).Find(&quotaDatas).Error
		return quotaDatas, err
	}

}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTime, endTime).Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetAllQuotaDates(startTime int64, endTime int64, username, tokenName string) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsername(username, tokenName, startTime, endTime)
	}
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	// only select model_name, sum(count) as count, sum(quota) as quota, model_name, created_at from quota_data group by model_name, created_at;
	//err = DB.Table("quota_data").Where("created_at >= ? and created_at <= ?", startTime, endTime).Find(&quotaDatas).Error
	err = DB.Table("quota_data").Select("model_name, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, created_at").Where("created_at >= ? and created_at <= ?", startTime, endTime).Group("model_name, created_at").Find(&quotaDatas).Error
	return quotaDatas, err
}

// GetAllQuotaDatesByDay 按天聚合查询数据
func GetAllQuotaDatesByDay(startTime int64, endTime int64, username, tokenName string) (quotaData []*QuotaDataByDay, err error) {
	var quotaDatas []*QuotaDataByDay

	// 构建查询，使用FROM_UNIXTIME按天聚合
	query := DB.Table("quota_data").
		Select("username, token_name, model_name, sum(count) as count, sum(quota)/500000 as price, sum(token_used) as token_used, FROM_UNIXTIME(created_at, '%Y-%m-%d') as date_str").
		Where("created_at >= ? and created_at <= ?", startTime, endTime)

	// 添加用户名和token名称过滤条件
	if username != "" {
		query = query.Where("username = ?", username)
	}
	if tokenName != "" {
		query = query.Where("token_name = ?", tokenName)
	}

	// 按天聚合分组
	err = query.Group("FROM_UNIXTIME(created_at, '%Y-%m-%d'), username, token_name, model_name").
		Find(&quotaDatas).Error

	return quotaDatas, err
}

func GetBilling(startTime int64, endTime int64, userName, tokenname string) (billingJsonData []*BillingJsonData, err error) {
	// 将时间戳转换为当天的开始时间（00:00:00）
	if endTime > time.Now().Unix() {
		endTime = time.Now().Unix()
	}
	currentTime := time.Unix(startTime, 0)
	currentTime = time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), 0, 0, 0, 0, currentTime.Location())
	endDateTime := time.Unix(endTime, 0)

	// 按天遍历时间范围
	for currentTime.Unix() <= endDateTime.Unix() {
		dayStart := currentTime.Unix()
		dayEnd := currentTime.Add(24 * time.Hour).Add(-time.Second).Unix()
		tableName := fmt.Sprintf("logs_%04d_%02d_%02d", currentTime.Year(), currentTime.Month(), currentTime.Day())
		if dayEnd > endTime {
			dayEnd = endTime
		}

		var billingData []*BillingData
		var tempBillingMap = make(map[string]*BillingData) // 用于临时存储聚合结果
		pageSize := 100000
		offset := 0

		for {
			var tempData []*struct {
				ChannelId        int
				ChannelName      string
				ChannelTag       string
				ModelName        string
				PromptTokens     int
				CompletionTokens int
			}

			if userName != "" {
				// 分页查询原始日志数据
				if tokenname != "" {
					err = DB.Table(tableName).
						Select(fmt.Sprintf("%s.channel_id, channels.name as channel_name, channels.tag as channel_tag, "+
																"%s.model_name, %s.prompt_tokens, %s.completion_tokens", tableName, tableName, tableName, tableName)).
						Joins(fmt.Sprintf("JOIN channels ON %s.channel_id = channels.id", tableName)). // 修复这里
						Where(fmt.Sprintf("%s.created_at BETWEEN ? AND ?", tableName), dayStart, dayEnd).
						Where(fmt.Sprintf("%s.username = ?", tableName), userName).
						Where(fmt.Sprintf("%s.token_name =?", tableName), tokenname).
						Order(fmt.Sprintf("%s.id", tableName)). // 修复这里
						Limit(pageSize).
						Offset(offset).
						Find(&tempData).Error
				} else {
					err = DB.Table(tableName).
						Select(fmt.Sprintf("%s.channel_id, channels.name as channel_name, channels.tag as channel_tag, "+
																"%s.model_name, %s.prompt_tokens, %s.completion_tokens", tableName, tableName, tableName, tableName)).
						Joins(fmt.Sprintf("JOIN channels ON %s.channel_id = channels.id", tableName)). // 修复这里
						Where(fmt.Sprintf("%s.created_at BETWEEN ? AND ?", tableName), dayStart, dayEnd).
						Where(fmt.Sprintf("%s.username = ?", tableName), userName).
						Order(fmt.Sprintf("%s.id", tableName)). // 修复这里
						Limit(pageSize).
						Offset(offset).
						Find(&tempData).Error
				}
			} else {
				// 分页查询原始日志数据
				err = DB.Table(tableName).
					Select(fmt.Sprintf("%s.channel_id, channels.name as channel_name, channels.tag as channel_tag, "+
															"%s.model_name, %s.prompt_tokens, %s.completion_tokens", tableName, tableName, tableName, tableName)).
					Joins(fmt.Sprintf("JOIN channels ON %s.channel_id = channels.id", tableName)). // 修复这里
					Where(fmt.Sprintf("%s.created_at BETWEEN ? AND ?", tableName), dayStart, dayEnd).
					Order(fmt.Sprintf("%s.id", tableName)). // 修复这里
					Limit(pageSize).
					Offset(offset).
					Find(&tempData).Error
			}

			if err != nil {
				return nil, err
			}

			// 如果没有更多数据，退出循环
			if len(tempData) == 0 {
				break
			}

			// 处理当前页的数据，进行内存聚合
			for _, item := range tempData {
				key := fmt.Sprintf("%s_%s", item.ChannelTag, item.ModelName)
				if _, ok := tempBillingMap[key]; !ok {
					tempBillingMap[key] = &BillingData{
						ChannelId:         item.ChannelId,
						ChannelName:       item.ChannelName,
						ChannelTag:        item.ChannelTag,
						ModelName:         item.ModelName,
						Count:             0,
						PromptTokens:      0,
						CompletionsTokens: 0,
					}
				}
				existing, _ := tempBillingMap[key]
				// 已存在的记录，累加计数
				existing.Count++
				existing.PromptTokens += item.PromptTokens
				existing.CompletionsTokens += item.CompletionTokens
			}

			offset += pageSize
		}

		// 将聚合结果转换为切片
		for _, data := range tempBillingMap {
			billingData = append(billingData, data)
		}

		sort.Slice(billingData, func(i, j int) bool {
			if billingData[i].ChannelTag != billingData[j].ChannelTag {
				return billingData[i].ChannelTag < billingData[j].ChannelTag
			} else if billingData[i].ChannelId != billingData[j].ChannelId {
				return billingData[i].ChannelId < billingData[j].ChannelId
			} else {
				return billingData[i].ModelName < billingData[j].ModelName
			}
		})

		// 处理当天的数据
		for _, data := range billingData {
			modelPrice1, ok1 := operation_setting.GetModelRatio(data.ModelName)
			modelPrice := 1.0

			if ok1 {
				modelPrice = modelPrice1
			}

			billingJsonData = append(billingJsonData, &BillingJsonData{
				ChannelId:          data.ChannelId,
				ChannelName:        data.ChannelName,
				ChannelTag:         data.ChannelTag,
				CurrentDate:        currentTime.Format("2006-01-02"),
				Count:              data.Count,
				ModelName:          data.ModelName,
				PromptTokens:       float32(data.PromptTokens),
				CompletionsTokens:  float32(data.CompletionsTokens),
				PromptPricing:      float32(modelPrice * 2),
				CompletionsPricing: float32(modelPrice * 2 * operation_setting.GetCompletionRatio(data.ModelName)),
				Cost:               (float32(data.PromptTokens)*float32(modelPrice*2) + float32(data.CompletionsTokens)*float32(modelPrice*2*operation_setting.GetCompletionRatio(data.ModelName))) / 100_0000,
			})
		}

		// 移动到下一天
		currentTime = currentTime.Add(24 * time.Hour)
	}

	// 在返回之前对数据进行排序
	sort.Slice(billingJsonData, func(i, j int) bool {
		// 首先按照 ChannelTag 排序
		if billingJsonData[i].ChannelTag != billingJsonData[j].ChannelTag {
			return billingJsonData[i].ChannelTag < billingJsonData[j].ChannelTag
		}
		// ChannelTag 相同时，按照 CurrentDate 排序
		return billingJsonData[i].CurrentDate < billingJsonData[j].CurrentDate
	})

	return billingJsonData, nil
}

func GetBillingAndExportExcel(startTime int64, endTime int64, userName string, tokenname string) ([]byte, error) {
	billingData, err := GetBilling(startTime, endTime, userName, tokenname)
	if err != nil {
		return nil, err
	}

	// 创建新的Excel文件
	f := excelize.NewFile()
	defer f.Close()

	// 设置表头
	headers := []string{"渠道Tag（Tag相同则聚合）", "日期", "调用次数", "模型名字",
		"提示Tokens", "补全Tokens", "提示价格", "补全价格", "金额"}
	for i, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue("Sheet1", cell, header)
		// 设置列宽为25
		f.SetColWidth("Sheet1", string('A'+i), string('A'+i), 25)
	}

	row := 2
	currentChannelTag := "null"
	var channelTotal float32 = 0

	// 在 GetBillingAndExportExcel 函数开始处添加样式定义
	style, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"FFD699"}, // 橙色
			Pattern: 1,
		},
	})
	if err != nil {
		return nil, err
	}

	// 写入数据
	for _, data := range billingData {
		// 如果是新的渠道ID，且不是第一条数据
		if currentChannelTag != "null" && currentChannelTag != data.ChannelTag && channelTotal > 0 {
			// 写入渠道总计行
			f.SetCellValue("Sheet1", fmt.Sprintf("A%d", row), currentChannelTag)
			f.SetCellValue("Sheet1", fmt.Sprintf("B%d", row), "总计")
			f.SetCellValue("Sheet1", fmt.Sprintf("C%d", row), "-")
			f.SetCellValue("Sheet1", fmt.Sprintf("D%d", row), "-")
			f.SetCellValue("Sheet1", fmt.Sprintf("E%d", row), "-")
			f.SetCellValue("Sheet1", fmt.Sprintf("F%d", row), "-")
			f.SetCellValue("Sheet1", fmt.Sprintf("G%d", row), "-")
			f.SetCellValue("Sheet1", fmt.Sprintf("H%d", row), "-")
			f.SetCellValue("Sheet1", fmt.Sprintf("I%d", row), channelTotal)
			// 为整行设置样式
			for col := 'A'; col <= 'I'; col++ {
				f.SetCellStyle("Sheet1", fmt.Sprintf("%c%d", col, row), fmt.Sprintf("%c%d", col, row), style)
			}
			row += 3
			channelTotal = 0
		}

		// 写入详细数据
		f.SetCellValue("Sheet1", fmt.Sprintf("A%d", row), data.ChannelTag)
		f.SetCellValue("Sheet1", fmt.Sprintf("B%d", row), data.CurrentDate)
		f.SetCellValue("Sheet1", fmt.Sprintf("C%d", row), data.Count)
		f.SetCellValue("Sheet1", fmt.Sprintf("D%d", row), data.ModelName)
		f.SetCellValue("Sheet1", fmt.Sprintf("E%d", row), data.PromptTokens)
		f.SetCellValue("Sheet1", fmt.Sprintf("F%d", row), data.CompletionsTokens)
		f.SetCellValue("Sheet1", fmt.Sprintf("G%d", row), data.PromptPricing)
		f.SetCellValue("Sheet1", fmt.Sprintf("H%d", row), data.CompletionsPricing)
		f.SetCellValue("Sheet1", fmt.Sprintf("I%d", row), data.Cost)

		channelTotal += data.Cost
		currentChannelTag = data.ChannelTag
		row++
	}

	// 写入最后一个渠道的总计行
	if channelTotal > 0 {
		f.SetCellValue("Sheet1", fmt.Sprintf("A%d", row), currentChannelTag)
		f.SetCellValue("Sheet1", fmt.Sprintf("B%d", row), "总计")
		f.SetCellValue("Sheet1", fmt.Sprintf("C%d", row), "-")
		f.SetCellValue("Sheet1", fmt.Sprintf("D%d", row), "-")
		f.SetCellValue("Sheet1", fmt.Sprintf("E%d", row), "-")
		f.SetCellValue("Sheet1", fmt.Sprintf("F%d", row), "-")
		f.SetCellValue("Sheet1", fmt.Sprintf("G%d", row), "-")
		f.SetCellValue("Sheet1", fmt.Sprintf("H%d", row), "-")
		f.SetCellValue("Sheet1", fmt.Sprintf("I%d", row), channelTotal)
		for col := 'A'; col <= 'I'; col++ {
			f.SetCellStyle("Sheet1", fmt.Sprintf("%c%d", col, row), fmt.Sprintf("%c%d", col, row), style)
		}
	}

	// 删除保存文件的代码，改为返回字节流
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

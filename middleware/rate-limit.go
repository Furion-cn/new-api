package middleware

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"one-api/common"
	"one-api/model"
	"time"

	"github.com/gin-gonic/gin"
)

var timeFormat = "2006-01-02T15:04:05.000Z"

var inMemoryRateLimiter common.InMemoryRateLimiter

// 返回限流错误（包括抢锁超时）
func returnRateLimitError(c *gin.Context, message string) {
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": gin.H{
			"code":    429,
			"message": message,
			"status":  "RATE_LIMIT_EXCEEDED",
		},
	})
	c.Abort()
}

var defNext = func(c *gin.Context) {
	c.Next()
}

func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	ctx := context.Background()
	rdb := common.RDB
	key := "rateLimit:" + mark + c.ClientIP()
	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if listLength < int64(maxRequestNum) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
		rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
	} else {
		oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		nowTimeStr := time.Now().Format(timeFormat)
		nowTime, err := time.Parse(timeFormat, nowTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		// time.Since will return negative number!
		// See: https://stackoverflow.com/questions/50970900/why-is-time-since-returning-negative-durations-on-windows
		if int64(nowTime.Sub(oldTime).Seconds()) < duration {
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		} else {
			rdb.LPush(ctx, key, time.Now().Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
		}
	}
}

func memoryRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	key := mark + c.ClientIP()
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

// redisRateLimiterWithWait 带等待机制的Redis限流器（使用Lua脚本保证原子性）
func redisRateLimiterWithWait(c *gin.Context, userId int, tokenName, modelName string) {
	// 构建限流标记：用户ID+token名+模型名
	mark := fmt.Sprintf("UTM:%d:%s:%s:", userId, tokenName, modelName)

	// 尝试处理请求（每次都会重新获取最新的限流配置）
	tryProcessRequest := func() bool {
		// 每次重试都重新获取最新的限流配置
		limit, err := model.GetUserTokenModelLimitWithCache(userId, tokenName, modelName)
		if err != nil {
			// 如果查询失败或没有配置，跳过限流
			return true
		}

		// 检查是否启用限流
		if !limit.RPMLimitEnabled {
			return true
		}

		// 检查RPM限制是否大于0
		if limit.RPMLimit <= 0 {
			return true
		}

		// 使用Lua脚本进行原子性限流检查
		ctx := context.Background()
		rdb := common.RDB
		key := "rateLimit:" + mark

		// Lua脚本：原子性地检查和处理限流
		luaScript := `
			local key = KEYS[1]
			local maxRequestNum = tonumber(ARGV[1])
			local duration = tonumber(ARGV[2])
			local currentTime = tonumber(ARGV[3])
			local expirationDuration = tonumber(ARGV[4])
			
			-- 获取当前列表长度
			local listLength = redis.call('LLEN', key)
			
			if listLength < maxRequestNum then
				-- 未达到限制，添加当前时间戳
				redis.call('LPUSH', key, currentTime)
				redis.call('EXPIRE', key, expirationDuration)
				return {1, 0} -- {allowed, waitTime}
			else
				-- 已达到限制，检查最旧的时间戳
				local oldestTimeStr = redis.call('LINDEX', key, -1)
				if oldestTimeStr == false then
					-- 列表为空，直接添加
					redis.call('LPUSH', key, currentTime)
					redis.call('EXPIRE', key, expirationDuration)
					return {1, 0}
				end
				
				-- 解析最旧时间戳
				local oldestTime = tonumber(oldestTimeStr)
				if oldestTime == nil then
					-- 时间戳格式错误，清理列表并添加新记录
					redis.call('DEL', key)
					redis.call('LPUSH', key, currentTime)
					redis.call('EXPIRE', key, expirationDuration)
					return {1, 0}
				end
				
				-- 计算时间差
				local timeDiff = currentTime - oldestTime
				
				if timeDiff < duration then
					-- 仍在时间窗口内，被限流
					redis.call('EXPIRE', key, expirationDuration)
					return {0, duration - timeDiff} -- {allowed, waitTime}
				else
					-- 时间窗口已过，移除最旧记录，添加新记录
					redis.call('LPUSH', key, currentTime)
					redis.call('LTRIM', key, 0, maxRequestNum - 1)
					redis.call('EXPIRE', key, expirationDuration)
					return {1, 0} -- {allowed, waitTime}
				end
			end
		`

		currentTime := time.Now().Unix()
		result, err := rdb.Eval(ctx, luaScript, []string{key},
			limit.RPMLimit, 60, currentTime, common.RateLimitKeyExpirationDuration.Seconds()).Result()

		if err != nil {
			fmt.Printf("Lua script error: %v\n", err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return false
		}

		// 解析结果
		resultSlice, ok := result.([]interface{})
		if !ok || len(resultSlice) != 2 {
			fmt.Printf("Invalid Lua script result: %v\n", result)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return false
		}

		allowed, ok1 := resultSlice[0].(int64)
		if !ok1 {
			fmt.Printf("Invalid Lua script result types: %v\n", resultSlice)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return false
		}

		return allowed == 1
	}

	// 如果立即成功处理，直接返回
	if tryProcessRequest() {
		return
	}

	// 被限流，开始等待和重试
	startTime := time.Now()

	// 获取等待时间配置
	limit, err := model.GetUserTokenModelLimitWithCache(userId, tokenName, modelName)
	if err != nil {
		// 如果无法获取配置，直接返回限流错误
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}

	waitDuration := time.Duration(limit.WaitDurationSeconds) * time.Second

	for {
		// 检查是否超过等待时间
		if time.Since(startTime) > waitDuration {
			// 返回抢锁超时错误
			returnRateLimitError(c, "Rate limit exceeded and failed to acquire RPM lock within timeout period")
			return
		}

		// 全局随机数生成器
		var rng = rand.New(rand.NewSource(time.Now().UnixNano()))
		// 随机等待时间：1-60秒之间，避免雷群效应
		waitSeconds := rng.Intn(60) + 1
		time.Sleep(time.Duration(waitSeconds) * time.Second)

		// 重新尝试处理请求（会重新获取最新的限流配置）
		if tryProcessRequest() {
			return // 成功处理
		}
	}
}

func rateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if common.RedisEnabled {
		return func(c *gin.Context) {
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	} else {
		// It's safe to call multi times.
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
		return func(c *gin.Context) {
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

// rateLimitFactoryWithWait 带等待机制的限流工厂函数（仅支持Redis）
func rateLimitFactoryWithWait(userId int, tokenName, modelName string) func(c *gin.Context) {
	if !common.RedisEnabled {
		// 如果Redis未启用，直接返回错误
		return func(c *gin.Context) {
			c.Status(http.StatusInternalServerError)
			c.Abort()
		}
	}

	return func(c *gin.Context) {
		redisRateLimiterWithWait(c, userId, tokenName, modelName)
	}
}

func GlobalWebRateLimit() func(c *gin.Context) {
	if common.GlobalWebRateLimitEnable {
		return rateLimitFactory(common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration, "GW")
	}
	return defNext
}

func GlobalAPIRateLimit() func(c *gin.Context) {
	if common.GlobalApiRateLimitEnable {
		return rateLimitFactory(common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration, "GA")
	}
	return defNext
}

func CriticalRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.CriticalRateLimitNum, common.CriticalRateLimitDuration, "CT")
}

func DownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.DownloadRateLimitNum, common.DownloadRateLimitDuration, "DW")
}

func UploadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.UploadRateLimitNum, common.UploadRateLimitDuration, "UP")
}

// UserTokenModelRateLimit 基于用户+token名+模型名的限流中间件
func UserTokenModelRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 获取用户信息
		userId := c.GetInt("id")
		if userId == 0 {
			c.Next()
			return
		}

		// 获取token信息
		tokenName := c.GetString("token_name")
		if tokenName == "" {
			c.Next()
			return
		}

		// 获取模型名称
		modelName := c.GetString("original_model")
		if modelName == "" {
			c.Next()
			return
		}

		// 从数据库获取限流配置
		limit, err := model.GetUserTokenModelLimitWithCache(userId, tokenName, modelName)
		if err != nil {
			// 如果查询失败或没有配置记录，默认不需要限流
			c.Next()
			return
		}

		// 检查是否启用限流（使用limit表中的RPMLimitEnabled字段）
		if !limit.RPMLimitEnabled {
			// 状态为false未开启，默认不需要限流
			c.Next()
			return
		}

		// 检查RPM限制是否大于0（使用limit表中的RPMLimit字段）
		if limit.RPMLimit <= 0 {
			// RPMLimit为0或负数，不需要限流
			c.Next()
			return
		}

		// 使用带等待机制的限流工厂，每次重试都会重新获取最新的限流配置
		rateLimitFunc := rateLimitFactoryWithWait(userId, tokenName, modelName)
		rateLimitFunc(c)
	}
}

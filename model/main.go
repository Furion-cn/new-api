package model

import (
	"log"
	"one-api/common"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var groupCol string
var keyCol string

func initCol() {
	if common.UsingPostgreSQL {
		groupCol = `"group"`
		keyCol = `"key"`

	} else {
		groupCol = "`group`"
		keyCol = "`key`"
	}
}

var DB *gorm.DB

var LOG_DB *gorm.DB

// 中心控制库数据库连接
var CENTRAL_DB *gorm.DB

func createRootAccountIfNeed() error {
	var user User
	//if user.Status != common.UserStatusEnabled {
	if err := DB.First(&user).Error; err != nil {
		common.SysLog("no user exists, create a root user for you: username is root, password is 123456")
		hashedPassword, err := common.Password2Hash("123456")
		if err != nil {
			return err
		}
		rootUser := User{
			Username:    "root",
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		DB.Create(&rootUser)
	}
	return nil
}

func chooseDB(envName string) (*gorm.DB, error) {
	defer func() {
		initCol()
	}()
	dsn := os.Getenv(envName)
	if dsn != "" {
		if strings.HasPrefix(dsn, "postgres://") {
			// Use PostgreSQL
			common.SysLog("using PostgreSQL as database")
			common.UsingPostgreSQL = true
			return gorm.Open(postgres.New(postgres.Config{
				DSN:                  dsn,
				PreferSimpleProtocol: true, // disables implicit prepared statement usage
			}), &gorm.Config{
				PrepareStmt: true, // precompile SQL
			})
		}
		if strings.HasPrefix(dsn, "local") {
			common.SysLog("SQL_DSN not set, using SQLite as database")
			common.UsingSQLite = true
			return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
				PrepareStmt: true, // precompile SQL
			})
		}
		// Use MySQL
		common.SysLog("using MySQL as database")
		// check parseTime
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		common.UsingMySQL = true
		return gorm.Open(mysql.Open(dsn), &gorm.Config{
			PrepareStmt: true, // precompile SQL
		})
	}
	// Use SQLite
	common.SysLog("SQL_DSN not set, using SQLite as database")
	common.UsingSQLite = true
	return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
		PrepareStmt: true, // precompile SQL
	})
}

func InitDB() (err error) {
	db, err := chooseDB("SQL_DSN")
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		DB = db
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 500))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 2000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		if common.UsingMySQL {
			_, _ = sqlDB.Exec("ALTER TABLE channels MODIFY model_mapping TEXT;") // TODO: delete this line when most users have upgraded
		}
		common.SysLog("database migration started")
		err = migrateDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func InitLogDB() (err error) {
	if os.Getenv("LOG_SQL_DSN") == "" {
		LOG_DB = DB
		return
	}
	db, err := chooseDB("LOG_SQL_DSN")
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		LOG_DB = db
		sqlDB, err := LOG_DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		//if common.UsingMySQL {
		//	_, _ = sqlDB.Exec("DROP INDEX idx_channels_key ON channels;")             // TODO: delete this line when most users have upgraded
		//	_, _ = sqlDB.Exec("ALTER TABLE midjourneys MODIFY action VARCHAR(40);")   // TODO: delete this line when most users have upgraded
		//	_, _ = sqlDB.Exec("ALTER TABLE midjourneys MODIFY progress VARCHAR(30);") // TODO: delete this line when most users have upgraded
		//	_, _ = sqlDB.Exec("ALTER TABLE midjourneys MODIFY status VARCHAR(20);")   // TODO: delete this line when most users have upgraded
		//}
		common.SysLog("database migration started")
		err = migrateLOGDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

// InitCentralDB 初始化中心控制库
func InitCentralDB() (err error) {
	if os.Getenv("CENTRAL_SQL_DSN") == "" {
		// 如果没有配置中心控制库，使用主数据库
		CENTRAL_DB = DB
		common.SysLog("CENTRAL_SQL_DSN not set, using main database for central control")
		return nil
	}
	db, err := chooseDB("CENTRAL_SQL_DSN")
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		CENTRAL_DB = db
		sqlDB, err := CENTRAL_DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		common.SysLog("central control database connected")
		return nil
	} else {
		common.FatalLog(err)
	}
	return err
}

func migrateDB() error {
	err := DB.AutoMigrate(&Channel{})
	if err != nil {
		return err
	}
	err = DB.AutoMigrate(&Token{})
	if err != nil {
		return err
	}
	err = DB.AutoMigrate(&User{})
	if err != nil {
		return err
	}
	err = DB.AutoMigrate(&Option{})
	if err != nil {
		return err
	}
	err = DB.AutoMigrate(&Redemption{})
	if err != nil {
		return err
	}
	err = DB.AutoMigrate(&Ability{})
	if err != nil {
		return err
	}
	err = DB.AutoMigrate(&Log{})
	if err != nil {
		return err
	}
	err = DB.AutoMigrate(&Midjourney{})
	if err != nil {
		return err
	}
	err = DB.AutoMigrate(&TopUp{})
	if err != nil {
		return err
	}
	err = DB.AutoMigrate(&QuotaData{})
	if err != nil {
		return err
	}
	err = DB.AutoMigrate(&Task{})
	if err != nil {
		return err
	}
	err = DB.AutoMigrate(&Group{})
	if err != nil {
		return err
	}
	common.SysLog("database migrated")
	err = createRootAccountIfNeed()
	return err
}

func migrateLOGDB() error {
	var err error
	if err = LOG_DB.AutoMigrate(&Log{}); err != nil {
		return err
	}
	return nil
}

func migrateCentralDB() error {
	// 迁移用户限速配置表
	err := CENTRAL_DB.AutoMigrate(&UserRateLimitConfig{})
	if err != nil {
		return err
	}

	common.SysLog("central control database migrated")
	return nil
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	return err
}

func CloseDB() error {
	if LOG_DB != DB {
		err := closeDB(LOG_DB)
		if err != nil {
			return err
		}
	}
	if CENTRAL_DB != DB && CENTRAL_DB != LOG_DB {
		err := closeDB(CENTRAL_DB)
		if err != nil {
			return err
		}
	}
	return closeDB(DB)
}

var (
	lastPingTime time.Time
	pingMutex    sync.Mutex
)

func PingDB() error {
	pingMutex.Lock()
	defer pingMutex.Unlock()

	if time.Since(lastPingTime) < time.Second*10 {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Error getting sql.DB from GORM: %v", err)
		return err
	}

	err = sqlDB.Ping()
	if err != nil {
		log.Printf("Error pinging DB: %v", err)
		return err
	}

	lastPingTime = time.Now()
	common.SysLog("Database pinged successfully")
	return nil
}

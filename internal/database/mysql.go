package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"goroutice/internal/config"
	"goroutice/internal/model"
	"goroutice/internal/pkg/hash"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewMySQL 建立 MySQL 连接并配置连接池。
func NewMySQL(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=%t&loc=%s",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.DBName, cfg.Charset, cfg.ParseTime, cfg.Loc)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Warn),
		DisableForeignKeyConstraintWhenMigrating: true,
		// 开启后驱动错误会被翻译为 gorm.ErrDuplicatedKey 等哨兵错误，
		// 否则唯一键冲突只能落到 500，无法映射为 409。
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	applyConnPool(sqlDB, cfg)

	return db, nil
}

// applyConnPool 设置连接池参数；未显式配置（值为 0）时根据 CPU 核数自动推导。
func applyConnPool(sqlDB *sql.DB, cfg *config.DatabaseConfig) {
	cpus := runtime.NumCPU()
	if cpus < 1 {
		cpus = 1
	}

	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = cpus
	}
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = cpus * 4
	}
	lifetimeMin := cfg.ConnMaxLifetimeMinutes
	if lifetimeMin <= 0 {
		lifetimeMin = 30
	}

	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetConnMaxLifetime(time.Duration(lifetimeMin) * time.Minute)
}

// AutoMigrate 自动创建/更新表结构。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{},
		&model.UserRole{},
		&model.Category{},
		&model.Tag{},
		&model.Article{},
		&model.File{},
		&model.PermissionAudit{},
		&model.RefreshToken{},
		&model.EmailVerification{},
		&model.PasswordReset{},
	)
}

// SeedAdmin 在管理员不存在时创建默认管理员账号。
func SeedAdmin(db *gorm.DB, username, password, email string) error {
	var count int64
	if err := db.Model(&model.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	// 口令改由环境变量注入后，「忘记配置」会变成用空口令创建管理员。
	// 仅在确实需要新建管理员时才拦截，已有管理员的实例不受影响。
	if password == "" {
		return errors.New("bootstrap.admin_password is empty: set BLOG_BOOTSTRAP_ADMIN_PASSWORD to create the initial admin")
	}

	hashed, err := hash.Password(password)
	if err != nil {
		return err
	}

	admin := &model.User{
		Username:     username,
		Email:        email,
		PasswordHash: hashed,
		Nickname:     "Administrator",
		Role:         model.RoleAdmin,
		Status:       model.StatusActive,
	}
	// 账号与角色必须同一事务写入：否则中断会留下「有账号无角色」的管理员，
	// 而下次启动会因账号已存在而跳过初始化，权限再也补不回来。
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(admin).Error; err != nil {
			return err
		}
		return tx.Create(&model.UserRole{UserID: admin.ID, Role: model.RoleAdmin}).Error
	})
	if err != nil {
		// 并发启动时另一实例已建好账号，视为成功。
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil
		}
		return err
	}

	slog.Warn("seeded default admin account, please change the password immediately",
		"username", username)
	return nil
}

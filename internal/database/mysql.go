package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
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
		&model.ArticleRevision{},
		&model.File{},
		&model.PermissionAudit{},
		&model.RefreshToken{},
		&model.EmailVerification{},
		&model.PasswordReset{},
	)
}

// fulltextIndexName 是文章表全文索引的名字，建索引与探测都以它为准。
const fulltextIndexName = "ft_article"

// articleFulltextColumns 必须与 ArticleRepository 里 MATCH(...) 的列完全一致：
// MySQL 要求 FULLTEXT 索引的列集合与 MATCH 参数完全匹配，否则直接报 1191 错误，
// 检索接口会从「慢」变成「500」。
var articleFulltextColumns = []string{"title", "summary", "content"}

// EnsureArticleFulltextIndex 确保文章表存在可用的全文索引，返回索引是否可用于检索。
//
// 两处刻意的选择：
//   - 用 ngram 解析器而非默认解析器。默认解析器按空格/标点分词，中文正文会被切成
//     一整段巨型 token，导致搜任何中文词都命中不了；
//   - 建索引失败不阻断启动，只降级为 LIKE 检索。数据库账号没有 ALTER 权限、
//     存储引擎不支持全文索引等情况下，检索会变慢但结果依然正确，比服务起不来更可取。
//
// 索引是否可用必须由这里探测后注入仓储层，不能只在仓储层按「驱动是不是 MySQL」判断：
// 驱动是 MySQL 但索引不存在时，MATCH 会直接报错。
func EnsureArticleFulltextIndex(db *gorm.DB) bool {
	if db.Dialector.Name() != "mysql" {
		// 测试用的 SQLite 没有 MySQL 的全文检索语法，检索走 LIKE 回退路径。
		return false
	}

	var count int64
	err := db.Raw(
		`SELECT COUNT(*) FROM information_schema.STATISTICS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?`,
		"articles", fulltextIndexName,
	).Scan(&count).Error
	if err != nil {
		slog.Warn("probe fulltext index failed, falling back to LIKE search", "err", err)
		return false
	}
	if count > 0 {
		return true
	}

	stmt := fmt.Sprintf("CREATE FULLTEXT INDEX %s ON articles (%s) WITH PARSER ngram",
		fulltextIndexName, strings.Join(articleFulltextColumns, ", "))
	if err := db.Exec(stmt).Error; err != nil {
		slog.Warn("create fulltext index failed, falling back to LIKE search", "err", err)
		return false
	}
	slog.Info("fulltext index created for article search", "index", fulltextIndexName)
	return true
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

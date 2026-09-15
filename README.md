# Goroutice Blog

基于 Go 的博客系统，提供用户认证、文章/分类/标签管理、文件上传、RBAC 权限管理等功能。采用分层架构（Handler → Service → Repository → Model），内置 JWT 鉴权、Casbin RBAC 授权、令牌桶限流、登录失败锁定、密码强度策略、权限审计与结构化日志等安全能力。

## 技术栈

| 类别 | 技术 |
| --- | --- |
| 语言 | Go 1.27 |
| Web 框架 | Gin |
| ORM | GORM（MySQL） |
| 权限 | Casbin（RBAC，基于 GORM 适配器） |
| 鉴权 | JWT（golang-jwt/v5）+ Refresh Token 轮换 |
| 配置 | Viper |
| 限流 | golang.org/x/time（令牌桶） |
| 密码 | bcrypt（golang.org/x/crypto） |
| 数据库 | MySQL（测试使用 SQLite） |

## 功能特性

- **用户与认证**：注册、登录（用户名或邮箱）、邮箱验证、忘记/重置密码、修改密码、注销、全端下线、注销账号。
- **内容管理**：文章（草稿/发布/归档、置顶、推荐、浏览量）、分类、标签，支持 slug 自动生成与唯一性处理。
- **全文检索**：MySQL `FULLTEXT`（`ngram` 解析器，支持中文）+ `MATCH ... AGAINST` 布尔检索；关键词统一归一化（切词、去重、限长限量、多词 AND），无全文索引时自动降级为转义后的 `LIKE`。
- **修订历史**：每次更新自动留档（整份快照，含正文/标签/分类），可按版本查看与一键回滚，回滚本身同样留档；按 `article.revision_keep` 自动裁剪。
- **订阅与索引**：`/feed.xml`（RSS 2.0）、`/sitemap.xml`（站点地图）、`/robots.txt`，均为根路径公开资源并带 10 分钟缓存头。
- **文件上传**：类型白名单、大小限制、UUID 重命名存储。
- **权限管理**：Casbin RBAC，角色继承 `admin → author → user`，权限策略、角色继承、用户角色的动态管理，并记录权限变更审计。
- **安全加固**：接口限流（按用户/IP/接口分桶、分档）、登录失败锁定、可配置密码强度策略、slug 合法性校验、超长字段与非法枚举校验。
- **可观测性**：RequestID 链路追踪、结构化日志、健康检查（存活/就绪）、优雅关闭。

## 快速开始

### 1. 前置条件

- Go 1.27+
- MySQL 8.0+（生产）或无需外部依赖（测试使用 SQLite）

### 2. 配置

复制并修改 `config.yaml`（非敏感项：端口、超时、限流、开关等），**密钥类配置不要写进该文件**，改由环境变量或 `.env` 注入：

```dotenv
# .env（已在 .gitignore 中，生产环境放在 server.exe 同目录）
BLOG_DATABASE_PASSWORD=your-db-password
BLOG_JWT_SECRET=replace-with-a-long-random-secret
```

```yaml
# config.yaml
database:
  host: 127.0.0.1
  port: 3306
  username: root
  password: "" # 环境变量 BLOG_DATABASE_PASSWORD

jwt:
  secret: "" # 环境变量 BLOG_JWT_SECRET
```

> 取值优先级：进程环境变量 > `.env` > `config.yaml` > 内置默认值；任意键都可用 `BLOG_` 前缀覆盖（如 `BLOG_DATABASE_HOST`、`BLOG_SERVER_TRUSTED_PROXIES`）。完整说明见 [docs/配置说明.md](docs/配置说明.md)。

### 3. 运行

```bash
# 安装依赖
go mod download

# 运行服务（会自动迁移表结构、初始化 Casbin 策略与默认管理员）
go run ./cmd/server

# 编译
go build -o bin/server ./cmd/server
```

服务默认监听 `http://localhost:8080`。首次启动时若库中还没有管理员，会用 `BLOG_BOOTSTRAP_ADMIN_PASSWORD`（环境变量或 `.env`）创建；该变量未设置则拒绝启动，以免用空口令建出管理员。

### 4. 验证

```bash
# 存活探针
curl http://localhost:8080/health/live

# 就绪探针
curl http://localhost:8080/health/ready

# 登录获取令牌（口令为 BLOG_BOOTSTRAP_ADMIN_PASSWORD 中设置的值）
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"account":"admin","password":"your-admin-password"}'
```

### 5. 测试与质量检查

```bash
# 运行单元测试
go test ./...

# 静态检查（golangci-lint）
golangci-lint run ./...
```

> 说明：服务层/仓库层测试使用纯 Go 的 SQLite 驱动（`github.com/glebarez/sqlite`），无需 CGO 与 C 编译器，`CGO_ENABLED=0` 下即可运行全部测试。

## 目录结构

```
goroutice/
├── cmd/server/             # 程序入口
├── internal/
│   ├── authz/              # Casbin 模型、策略种子与 GORM 适配器
│   ├── config/             # 配置加载
│   ├── database/           # 数据库连接、迁移、管理员种子
│   ├── dto/                # 请求/响应数据传输对象
│   ├── handler/            # HTTP 处理器层
│   ├── middleware/         # 中间件（认证/授权/限流/日志等）
│   ├── model/              # 数据模型
│   ├── pkg/
│   │   ├── apperror/       # 业务错误
│   │   ├── hash/           # bcrypt 密码哈希
│   │   ├── jwt/            # JWT 签发与解析
│   │   ├── pagination/     # 分页参数
│   │   ├── response/       # 统一响应结构
│   │   ├── search/         # 检索关键词归一化（全文检索 / LIKE 共用）
│   │   ├── slug/           # slug 生成与校验
│   │   └── token/          # 随机令牌生成与哈希
│   ├── repository/         # 数据访问层
│   ├── router/             # 路由注册
│   └── service/            # 业务逻辑层
├── docs/                   # 项目文档
├── uploads/                # 上传文件存储目录
├── config.yaml             # 配置文件
└── go.mod
```

## 文档导航

- [API 接口文档](docs/API.md)：全部接口、请求/响应示例、错误码与数据模型。
- [配置说明](docs/配置说明.md)：`config.yaml` 全字段说明与环境变量覆盖。
- [架构与设计](docs/架构与设计.md)：分层架构、认证授权机制、数据库表结构与中间件。


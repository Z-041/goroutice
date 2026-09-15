# API 接口文档

本文档描述 Goroutice Blog 的全部 HTTP 接口，包括请求/响应格式、鉴权方式、角色权限与错误码。

- 文档基准：[router.go](file:///d:/Project/Go/goroutice/internal/router/router.go) 中的实际路由与各 handler 的响应结构。
- 修改接口后请同步更新本文档。

## 目录

- [基础约定](#基础约定)
- [角色与权限矩阵](#角色与权限矩阵)
- [健康检查](#健康检查)
- [认证与账户](#认证与账户)
- [公开内容](#公开内容)
- [需登录接口](#需登录接口)
- [管理员接口](#管理员接口)
- [订阅与站点文件](#订阅与站点文件)
- [静态资源](#静态资源)
- [数据模型与枚举](#数据模型与枚举)
- [前端对接注意事项](#前端对接注意事项)
- [升级注意事项](#升级注意事项)

---

## 基础约定

### Base URL

```
http://localhost:8080
```

所有业务接口均以 `/api/v1` 为前缀；健康检查与静态资源除外。

### 请求头

| 请求头 | 必填 | 说明 |
| --- | --- | --- |
| `Content-Type` | 写接口必填 | `application/json`；文件上传为 `multipart/form-data` |
| `Authorization` | 需登录接口必填 | `Bearer <access_token>`，`Bearer` 大小写不敏感 |
| `X-Request-Id` | 否 | 自定义请求追踪 ID；不传则服务端生成 UUID |

### 响应头

| 响应头 | 说明 |
| --- | --- |
| `X-Request-Id` | 本次请求的追踪 ID（回显或服务端生成） |
| `Access-Control-Allow-Origin` 等 | CORS 响应头，见下方 CORS 说明 |

**CORS**：允许来源由配置 `cors.allowed_origins` 决定（默认 `*`）；允许方法 `GET/POST/PUT/PATCH/DELETE/OPTIONS`；允许请求头 `Origin`、`Content-Type`、`Authorization`、`X-Request-Id`；暴露响应头 `Content-Length`、`X-Request-Id`；`AllowCredentials=false`；预检缓存 12 小时。

### 鉴权

需要登录的接口通过请求头携带访问令牌：

```
Authorization: Bearer <access_token>
```

- `access_token` 为 JWT，默认 15 分钟过期（`jwt.access_expire_minutes`）。
- `refresh_token` 为随机字符串，默认 720 小时（30 天）过期（`jwt.refresh_expire_hours`），服务端仅存哈希。
- 过期后用 `refresh_token` 调 `/auth/refresh` 换取新令牌对；**旧 refresh token 立即吊销**（一次性轮换）。
- 以下情况即使 access token 未过期也会返回 401：用户不存在、用户状态非「正常」、`token_version` 不匹配（改密/重置密码/全端下线/注销后旧令牌全部失效）。

### 统一响应格式

成功响应（HTTP 200/201）：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

失败响应（HTTP 4xx/5xx）：

```json
{
  "code": 400,
  "message": "错误信息"
}
```

- `code`：`0` 表示成功；非 `0` 时等于 HTTP 状态码。
- `message`：成功固定为 `success`；失败为英文错误描述（可用于提示，但不建议用于逻辑分支）。
- `data`：成功时携带业务数据；**当业务数据为空（如删除、登出）时该字段会被完全省略**；失败时同样省略。
- 参数校验失败统一返回 `400 invalid request body`，不返回具体字段级错误。

### 分页

列表接口支持分页查询参数：

| 参数 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `page` | int | 1 | 页码，从 1 开始；非法值或 `小于 1` 时回退为 1 |
| `size` | int | 10 | 每页条数，最大 100；非法值或 `小于 1` 时回退为 10，大于 100 时截断为 100 |

> 分页参数不做报错处理，只会被静默修正。

分页响应 `data`：

```json
{
  "list": [],
  "total": 100,
  "page": 1,
  "size": 10
}
```

### 时间格式

所有时间字段使用 RFC3339 格式，例如 `2026-09-13T12:00:00+08:00`。

### 请求体大小限制

全局限流中间件按 `server.max_body_size`（默认 16MB）限制请求体：

- 带 `Content-Length` 且超限：直接返回 `413 request body too large`。
- 无 `Content-Length`（分块传输）：读取超过上限后中断。

### 限流

按「身份 + 路由」维度分桶（已登录用用户 ID，未登录用客户端 IP）：

| 档位 | 默认值 | 作用范围 |
| --- | --- | --- |
| 默认档 | 5 rps / 突发 10 | `POST /auth/register`、`/auth/login`、`/auth/refresh`、`/auth/resend-verification`、`/auth/forgot-password`、`/auth/reset-password` |
| 写档 | 20 rps / 突发 40 | 所有需登录分组内的**写方法**（POST/PUT/PATCH/DELETE），GET 不经过写档 |

超限返回 `429 too many requests`。

> `POST /auth/logout`、`POST /auth/verify-email` 不受限流中间件保护。

### 错误码

| 状态码 | 含义 | 常见场景 |
| --- | --- | --- |
| 400 | 参数错误 | 请求体校验失败、密码强度不足、令牌无效/已用/过期、非法 slug、关联的分类或标签不存在 |
| 401 | 未认证 | 缺少/格式错误的 Authorization 头、令牌无效或过期、refresh token 无效或过期（加载账号时数据库故障属服务端故障，返回 500） |
| 403 | 无权限 | 角色不满足策略、非本人资源、账号被禁用、邮箱未验证 |
| 404 | 资源不存在 | 目标记录不存在；公开接口访问未发布文章同样返回 404 |
| 409 | 冲突 | 用户名/邮箱重复、分类名/标签名重复、策略或角色继承已存在、分类被文章引用、文章内容被他人抢先修改（乐观锁，见 `PUT /api/v1/articles/:id`） |
| 413 | 请求体过大 | 超过 `server.max_body_size` |
| 429 | 请求过于频繁 | 触发限流或登录失败锁定 |
| 500 | 服务内部错误 | 未预期异常、panic 恢复、Casbin 执行异常 |
| 503 | 服务不可用 | 就绪检查失败（数据库不可用） |

---

## 角色与权限矩阵

角色为多值（`roles` 数组），任一角色通过策略即放行；继承关系：`admin` → `author` → `user`。

| 接口 | 方法 | 最低角色 | 额外约束 |
| --- | --- | --- | --- |
| `/api/v1/auth/profile` | GET / PUT | `user` | 仅本人 |
| `/api/v1/auth/password` | PUT | `user` | 仅本人 |
| `/api/v1/auth/logout-all` | POST | `user` | 仅本人，吊销该用户全部 refresh token |
| `/api/v1/auth/me` | DELETE | `user` | 仅本人，注销（软删除）当前账号 |
| `/api/v1/articles` | POST | `author` | — |
| `/api/v1/articles/:id` | PUT / DELETE | `author` | 仅作者本人或 `admin` |
| `/api/v1/me/articles`、`/me/articles/:id` | GET | `author` | 仅本人文章；`admin` 可查任意 |
| `/api/v1/me/articles/:id/revisions(/:revisionId)` | GET | `author` | 仅本人文章；`admin` 可查任意 |
| `/api/v1/me/articles/:id/revisions/:revisionId/restore` | POST | `author` | 仅本人文章；`admin` 可操作任意 |
| `/api/v1/files` | POST / GET | `author` | GET 非 `admin` 仅返回本人文件 |
| `/api/v1/files/:id` | DELETE | `author` | 仅上传者本人或 `admin` |
| `/api/v1/admin/**` | 全部 | `admin` | 路径前缀匹配 |

公开接口（无需令牌）：`/health/*`、`/uploads/*`、`/feed.xml`、`/sitemap.xml`、`/robots.txt`、`GET /api/v1/categories(/:id)`、`GET /api/v1/tags(/:id)`、`GET /api/v1/articles(/:key)`。

鉴权失败顺序：未带/无效令牌 → `401`；令牌合法但角色不匹配 → `403 forbidden`。

---

## 健康检查

### GET `/health/live`

存活探针，仅表示进程存活，不依赖外部服务。

**响应**

```json
{ "code": 0, "message": "success", "data": { "status": "up" } }
```

### GET `/health/ready`

就绪探针，检查数据库连通性。

**响应**

成功：

```json
{ "code": 0, "message": "success", "data": { "status": "ready" } }
```

失败（503）：

```json
{ "code": 503, "message": "database unavailable" }
```

---

## 认证与账户

> 本节的接口若未特别说明，均无需 `Authorization` 头。带 🔒 的小节需登录。

### POST `/api/v1/auth/register`

注册新用户。启用邮箱验证时（`security.email_verification_enabled=true`），注册后用户状态为「未验证」（`status=2`），需调用邮箱验证接口激活。

**限流**：默认档。

**请求体**

```json
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "secret123",
  "nickname": "Alice"
}
```

| 字段 | 类型 | 必填 | 约束 |
| --- | --- | --- | --- |
| `username` | string | 是 | 3–32 字符，全局唯一 |
| `email` | string | 是 | 合法邮箱，≤128，全局唯一 |
| `password` | string | 是 | 8–64，且满足密码策略（见下） |
| `nickname` | string | 否 | ≤64，缺省为用户名 |

**密码策略**（`security.*` 可配，默认仅要求长度与字母+数字）：

- 长度 ≥ `password_min_length`（默认 8）。
- 必须**同时包含字母与数字**。
- `password_require_uppercase=true` 时须含大写字母。
- `password_require_special=true` 时须含特殊字符。

**响应（201）**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "uuid",
    "username": "alice",
    "email": "alice@example.com",
    "nickname": "Alice",
    "avatar": "",
    "bio": "",
    "role": "user",
    "roles": ["user"],
    "status": 2,
    "created_at": "2026-09-13T12:00:00+08:00"
  }
}
```

> `status` 取值：`0` 禁用、`1` 正常、`2` 未验证。

**常见错误**

- `400 invalid request body`：字段缺失或长度/邮箱格式不合法。
- `400 password must be at least 8 characters` / `password must contain both letters and digits` / `password must contain an uppercase letter` / `password must contain a special character`：密码强度不足。
- `409 username already exists` / `email already exists` / `username or email already exists`。
- `429 too many requests`。

### POST `/api/v1/auth/login`

用户登录，`account` 可为用户名或邮箱。

**限流**：默认档（连续失败受登录锁定限制，默认 5 次 / 15 分钟）。

**请求体**

```json
{
  "account": "alice",
  "password": "secret123"
}
```

| 字段 | 类型 | 必填 | 约束 |
| --- | --- | --- | --- |
| `account` | string | 是 | ≤128 |
| `password` | string | 是 | ≤64 |

**响应（200）**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "tokens": {
      "access_token": "eyJhbGciOi...",
      "access_expires_at": "2026-09-13T12:15:00+08:00",
      "refresh_token": "随机字符串",
      "refresh_expires_at": "2026-10-13T12:00:00+08:00"
    },
    "user": {
      "id": "uuid",
      "username": "alice",
      "email": "alice@example.com",
      "nickname": "Alice",
      "avatar": "",
      "bio": "",
      "role": "user",
      "roles": ["user"],
      "status": 1,
      "created_at": "2026-09-13T12:00:00+08:00"
    }
  }
}
```

**常见错误**

- `401 invalid account or password`：账号不存在或密码错误（不区分，避免枚举账号）。
- `403 account is disabled`：账号被禁用。
- `403 email not verified`：邮箱未验证。
- `429 too many failed attempts, please try again later`：连续登录失败触发锁定。
- `429 too many requests`：触发限流。

### POST `/api/v1/auth/refresh`

使用 refresh token 轮换令牌对，旧 refresh token 立即吊销。

**限流**：默认档。

**请求体**

```json
{ "refresh_token": "随机字符串" }
```

**响应（200）**

> 注意：`data` 直接是令牌对，**不嵌套在 `tokens` 下，也不返回 `user`**。

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "eyJhbGciOi...",
    "access_expires_at": "2026-09-13T12:15:00+08:00",
    "refresh_token": "新的随机字符串",
    "refresh_expires_at": "2026-10-13T12:00:00+08:00"
  }
}
```

**常见错误**

- `401 invalid refresh token`：令牌不存在（可能已使用或已吊销）。
- `401 refresh token expired`：令牌已过期。
- `401 user not found`：关联用户已被删除。
- `403 account is disabled`：用户状态非「正常」。

### POST `/api/v1/auth/logout`

登出，吊销单个 refresh token。幂等：令牌不存在时同样返回成功。

**请求体**

```json
{ "refresh_token": "随机字符串" }
```

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

### POST `/api/v1/auth/verify-email`

校验邮箱验证令牌并激活账号。

**请求体**

```json
{ "token": "验证令牌" }
```

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**

- `400 invalid token`：令牌不存在。
- `400 token already used`：令牌已被使用。
- `400 token expired`：令牌已过期（有效期 24 小时）。
- `400 email already verified`：账号已处于已验证状态。
- `404 user not found`：令牌关联用户不存在。

### POST `/api/v1/auth/resend-verification`

重新发送邮箱验证邮件（仅对「未验证」状态用户有效）。
为避免泄露账号是否存在，邮箱未注册时同样返回成功。

**限流**：默认档。

**请求体**

```json
{ "email": "alice@example.com" }
```

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**

- `400 email already verified`：账号已验证。

### POST `/api/v1/auth/forgot-password`

发送密码重置邮件。为避免泄露账号是否存在，邮箱未注册时同样返回成功。

**限流**：默认档。

**请求体**

```json
{ "email": "alice@example.com" }
```

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

> 重置令牌有效期 1 小时。

### POST `/api/v1/auth/reset-password`

校验重置令牌并设置新密码，同时使旧登录态全部失效（`token_version` 递增 + 吊销全部 refresh token）。

**限流**：默认档。

**请求体**

```json
{
  "token": "重置令牌",
  "new_password": "newsecret123"
}
```

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：同 `/auth/verify-email` 的令牌类错误（`invalid token`、`token already used`、`token expired`），以及密码强度错误与 `404 user not found`。

---

🔒 以下接口需要 `Authorization: Bearer <access_token>` 请求头，最低角色 `user`。

### GET `/api/v1/auth/profile`

获取当前登录用户资料。

**响应（200）**：`data` 为用户信息对象（结构同注册响应，`roles` 为该用户完整角色列表）。

### PUT `/api/v1/auth/profile`

更新当前用户资料。

**请求体**

```json
{
  "email": "new@example.com",
  "nickname": "New Nick",
  "avatar": "https://example.com/avatar.png",
  "bio": "个人简介"
}
```

| 字段 | 类型 | 必填 | 约束 | 省略时的行为 |
| --- | --- | --- | --- | --- |
| `email` | string | 否 | 合法邮箱，≤128，需唯一 | 保持原值 |
| `nickname` | string | 否 | ≤64 | 保持原值 |
| `avatar` | string | 否 | ≤255 | ⚠️ 被置为空串 |
| `bio` | string | 否 | ≤255 | ⚠️ 被置为空串 |

> `avatar` 与 `bio` 是无条件覆盖写，只想改昵称时请务必回传这两个字段的原值，否则会被清空。

**响应（200）**：返回更新后的用户信息。

**常见错误**

- `409 email already exists`：新邮箱已被占用。
- `404 user not found`。

> 修改邮箱不会重新触发邮箱验证，状态也不会变为「未验证」。

### PUT `/api/v1/auth/password`

修改密码，成功后 `token_version` 递增并吊销全部 refresh token（旧登录态全部失效，需重新登录）。

**请求体**

```json
{
  "old_password": "secret123",
  "new_password": "newsecret123"
}
```

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**

- `400 old password is incorrect`。
- `400` 密码强度错误（同注册）。

### POST `/api/v1/auth/logout-all`

全端下线：递增令牌版本并吊销所有 refresh token。**无需请求体**，目标用户取自当前令牌。

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

调用后该用户已签发的全部 access token 立即失效（`401 invalid or expired token`），需要重新登录。

### DELETE `/api/v1/auth/me`

注销当前账号（软删除，清理角色、全部 refresh token 与验证/重置令牌）。**无需请求体**。

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

---

## 公开内容

以下接口无需鉴权，仅返回已发布（`published`）的文章。

### GET `/api/v1/categories`

分页查询分类。

**查询参数**：`page`、`size`、`keyword`（按名称模糊搜索）。

**响应（200）**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [
      {
        "id": "uuid",
        "name": "Go",
        "slug": "go",
        "description": "Go 语言相关",
        "sort": 0,
        "created_at": "2026-09-13T12:00:00+08:00"
      }
    ],
    "total": 1,
    "page": 1,
    "size": 10
  }
}
```

### GET `/api/v1/categories/:id`

按 ID 查询分类详情。

**响应（200）**：`data` 为分类对象（结构同上）。

**常见错误**：`404 category not found`。

### GET `/api/v1/tags`

分页查询标签。

**查询参数**：`page`、`size`、`keyword`。

**响应（200）**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [
      { "id": "uuid", "name": "并发", "slug": "concurrency", "created_at": "2026-09-13T12:00:00+08:00" }
    ],
    "total": 1,
    "page": 1,
    "size": 10
  }
}
```

### GET `/api/v1/tags/:id`

按 ID 查询标签详情。

**响应（200）**：`data` 为标签对象。

**常见错误**：`404 tag not found`。

### GET `/api/v1/articles`

公开文章列表（仅 `published`）。

**查询参数**

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `page` / `size` | int | 分页 |
| `keyword` | string | 按标题/摘要/正文检索（见下） |
| `category_id` | string | 按分类过滤 |
| `tag_id` | string | 按标签过滤 |

> **检索语义**：关键词先被归一化——只保留字母/数字（含中文），按非字母数字字符切词，去重、单 term 最长 32 字符、最多取 8 个 term，词之间为「同时命中」的 AND 语义。例如 `go 并发` 与 `go, 并发` 等价。
> 数据库存在 `ft_article` 全文索引（MySQL 需 `ngram` 解析器）时走 `MATCH ... AGAINST`；此时若关键词不含任何有效 term（如仅输入 `%`、`+`），直接返回空列表，而不是命中全部文章。
> 无全文索引时回退为 `LIKE` 子串匹配，`%`、`_`、`\` 会被转义为字面量（不会退化成通配符）。

**排序**：`is_pinned DESC, is_featured DESC, created_at DESC`。

**响应（200）**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [
      {
        "id": "uuid",
        "title": "文章标题",
        "slug": "article-slug",
        "summary": "摘要",
        "cover_image": "",
        "status": "published",
        "view_count": 10,
        "is_pinned": false,
        "is_featured": false,
        "category_id": "uuid",
        "category": { "id": "uuid", "name": "Go", "slug": "go", "description": "", "sort": 0, "created_at": "2026-09-13T12:00:00+08:00" },
        "tags": [ { "id": "uuid", "name": "并发", "slug": "concurrency", "created_at": "2026-09-13T12:00:00+08:00" } ],
        "author_id": "uuid",
        "author": { "id": "uuid", "username": "alice", "nickname": "Alice", "avatar": "", "bio": "", "role": "user", "roles": null, "status": 1, "created_at": "2026-09-13T12:00:00+08:00" },
        "published_at": "2026-09-13T12:00:00+08:00",
        "created_at": "2026-09-13T12:00:00+08:00",
        "updated_at": "2026-09-13T12:00:00+08:00"
      }
    ],
    "total": 1,
    "page": 1,
    "size": 10
  }
}
```

> 列表响应为「摘要」结构，不含 `content` 字段。
> `category`、`tags`、`author`、`published_at` 为 `omitempty`：为空时字段**不存在**（而非 `null`），前端需做缺省处理。嵌套的 `author.roles` 恒为 `null`。
> 文章接口（含公开列表/详情与管理端）的作者信息**不下发 `author.email`**（账号 PII，字段直接不存在）；管理端需要邮箱请走 `/api/v1/admin/users`。

### GET `/api/v1/articles/:key`

公开文章详情，`key` 可为文章 ID（UUID）或 slug。

**响应（200）**：`data` 为文章详情对象（含 `content` 与 `version`，其余字段同上）。

> `view_count` 按**来源去重**后自增：同一来源（IP + `User-Agent`）在该文章的去重窗口内重复打开只计一次，返回体中的 `view_count` 已是本次自增后的值（未计入本次时返回当前值）。
> 去重窗口由 `article.view_dedup_minutes` 配置（默认 30 分钟），改用另一个浏览器（UA 不同）或换网络（IP 不同）都会计为新的一次。
> 该机制用于抑制刷新与爬虫造成的失真，**不是安全边界**：刻意伪造 `User-Agent` 仍可绕过。

**常见错误**：`404 article not found`（不存在**或未发布**均返回 404）。

---

## 需登录接口

以下接口需要认证，最低角色 `author`（`user` 角色无写权限）。

### POST `/api/v1/files`

上传文件（`multipart/form-data`）。

**请求**

- 表单字段：`file`（必填，文件本体）。

**约束**

- 大小 ≤ `upload.max_size`（默认 10MB）。
- 扩展名需在 `upload.allowed_exts` 白名单内（默认 `.jpg/.jpeg/.png/.gif/.webp/.svg/.pdf/.txt/.md/.zip`），按小写比较。
- 实际落盘文件名为 `<uuid><ext>`，原始文件名仅作为 `name` 记录。

**响应（201）**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "uuid",
    "name": "photo.png",
    "url": "/uploads/<uuid>.png",
    "mime_type": "image/png",
    "size": 1024,
    "uploader_id": "uuid",
    "created_at": "2026-09-13T12:00:00+08:00"
  }
}
```

**常见错误**

- `400 file is required`：缺少 `file` 表单字段。
- `400 file size exceeds limit`：超过大小上限。
- `400 file type not allowed`：扩展名不在白名单。

### GET `/api/v1/files`

分页查询文件。非 `admin` 仅能查看自己上传的文件；`admin` 可查看全部。

**查询参数**：`page`、`size`。

**响应（200）**：`data.list` 为文件对象数组（结构同上），按创建时间倒序。

### DELETE `/api/v1/files/:id`

删除文件。仅上传者本人或 `admin` 可操作；同时尽力删除物理文件（失败不影响响应）。

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**

- `404 file not found`。
- `403 you can only delete your own files`。

### GET `/api/v1/me/articles`

当前作者的文章列表（含草稿，不含他人文章）。

**查询参数**：`page`、`size`、`status`（可选：`draft`/`published`/`archived`）。

> `status` 不做枚举校验，传非法值时按等值过滤，返回空列表。

**响应（200）**：`data.list` 为文章摘要数组，排序同公开列表。

### GET `/api/v1/me/articles/:id`

当前作者的文章详情（任意状态，含 `content`）。

**响应（200）**：`data` 为文章详情对象。

**常见错误**

- `404 article not found`。
- `403 you can only view your own articles`。

### POST `/api/v1/articles`

创建文章。作者为当前登录用户。

**请求体**

```json
{
  "title": "文章标题",
  "slug": "my-slug",
  "summary": "摘要",
  "content": "正文内容",
  "cover_image": "https://example.com/cover.png",
  "status": "draft",
  "category_id": "uuid",
  "tag_ids": ["uuid1", "uuid2"]
}
```

| 字段 | 类型 | 必填 | 约束 |
| --- | --- | --- | --- |
| `title` | string | 是 | ≤255 |
| `slug` | string | 否 | ≤255，仅小写字母/数字/连字符，且不能以连字符开头或结尾 |
| `summary` | string | 否 | ≤500 |
| `content` | string | 是 | ≤1048576（1MB） |
| `cover_image` | string | 否 | ≤255 |
| `status` | string | 否 | `draft`/`published`/`archived`，缺省为 `draft` |
| `category_id` | string | 否 | 必须为已存在分类 ID |
| `tag_ids` | []string | 否 | 必须全部为已存在标签 ID；自动去重、忽略空串 |
| `version` | int | 否 | 乐观锁版本号（≥0）；创建时忽略 |

**行为说明**

- `slug` 为空时由 `title` 自动生成（中文等非 ASCII 标题会退化为 8 位随机串）。
- slug 冲突不报错，自动追加 `-2`、`-3` 等后缀。
- `status=published` 时首次写入 `published_at`。
- 新建文章的 `version` 恒为 `1`（随详情返回）。

**响应（201）**：`data` 为文章详情对象。

**常见错误**

- `400 invalid slug`。
- `400 category not found` / `400 one or more tags not found`。

### PUT `/api/v1/articles/:id`

全量更新文章（作者本人或 `admin`）。

**请求体**：同创建文章。

> 除 `slug`（为空或与原值相同则不改动）与 `status`（为空则保持原状态）外，其余字段均为覆盖写；`tag_ids` 为空数组时会清空标签关联。

**乐观锁（`version`）**

请求体里的 `version` 是**编辑前从详情接口读到的版本号**，用于避免覆盖他人已提交的修改：

- 传 `0` 或不传：不做并发检查（兼容尚未接入该字段的客户端）。
- 传非 0 且与库中版本不一致：**拒绝写入并返回 `409`**，返回体不含 `data`，调用方应重新拉取详情、合并改动后再提交。
- 写入成功后 `version` 自增 1，响应体里是最新值。

> 版本号在**文章内容被改动时**推进，包括 `PUT /api/v1/articles/:id`（含回滚）与 `PUT /api/v1/admin/articles/:id/status`。
> 因此管理员改了状态后，作者用打开编辑页时的旧 `version` 提交同样会得到 `409`——这正是要拦住的情况。
> `PUT /api/v1/admin/articles/:id/feature`（置顶/推荐）**不**推进版本：这两个字段不属于 `ArticleRequest`，`PUT` 覆盖不到它们，推进只会带来无从处理的 `409`。

**响应（200）**：`data` 为更新后的文章详情。

**常见错误**

- `404 article not found`。
- `403 you can only modify your own articles`。
- `400 invalid slug` / `400 category not found` / `400 one or more tags not found`。
- `409 article was modified by someone else, reload and retry`。

### DELETE `/api/v1/articles/:id`

软删除文章（作者本人或 `admin`）。

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：`404 article not found`、`403 you can only delete your own articles`。

### GET `/api/v1/me/articles/:id/revisions`

分页查询某篇文章的修订历史（作者本人或 `admin`），按 `version` 倒序。

**查询参数**：`page`、`size`。

**响应（200）**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [
      {
        "id": "uuid",
        "article_id": "uuid",
        "editor_id": "uuid",
        "version": 3,
        "title": "文章标题",
        "slug": "article-slug",
        "status": "published",
        "created_at": "2026-09-13T12:00:00+08:00"
      }
    ],
    "total": 1,
    "page": 1,
    "size": 10
  }
}
```

> 列表**不含正文**（`content`），只用于展示版本时间线。
> 每篇文章最多保留 `article.revision_keep` 条（默认 20），超出部分在写入时按 `version` 从小到大裁剪。

**常见错误**：`404 article not found`、`403 you can only modify your own articles`。

### GET `/api/v1/me/articles/:id/revisions/:revisionId`

修订详情（含正文），作者本人或 `admin`。

**响应（200）**：`data` 为修订详情对象（含 `summary`、`content`、`cover_image`、`category_id`、`tag_ids`）。

**常见错误**：`404 article not found`、`404 revision not found`、`403 you can only modify your own articles`。

> 修订 ID 必须**属于该文章**，否则同样返回 `404 revision not found`（避免用任意修订 ID 读取他人内容）。

### POST `/api/v1/me/articles/:id/revisions/:revisionId/restore`

把文章回滚到指定修订（作者本人或 `admin`）。无请求体。

> 回滚本身也会写入一条修订（记录回滚前的内容），因此**回滚可以被再次回滚**。
> 回滚会连同 `title`、`slug`、`summary`、`content`、`cover_image`、`status`、`category_id`、`tag_ids` 一起恢复为快照内容。
> 该接口没有请求体，乐观锁由服务端用刚读到的版本号施加：能拦住「读取之后、写入之前被他人改动」的情况，冲突时返回 `409`；重新调用即可（回滚是幂等的）。

**响应（200）**：`data` 为回滚后的文章详情对象。

**副作用**：写入审计日志 `article_restore`（`article=<slug> revision=<revisionId>`）。

**常见错误**：`404 article not found`、`404 revision not found`、`403 you can only modify your own articles`、`409 article was modified by someone else, reload and retry`。

---

## 管理员接口

以下接口需要 `admin` 角色，路径前缀 `/api/v1/admin`（策略为 `/api/v1/admin/*`，方法不限）。

### GET `/api/v1/admin/users`

分页查询用户。

**查询参数**：`page`、`size`、`keyword`（按用户名/邮箱/昵称模糊搜索）。

**响应（200）**：`data.list` 为用户信息对象数组（含 `roles`）。

### PUT `/api/v1/admin/users/:id`

更新用户（禁用/启用、昵称）。

**请求体**

```json
{
  "status": 0,
  "nickname": "新昵称"
}
```

| 字段 | 类型 | 必填 | 约束 |
| --- | --- | --- | --- |
| `status` | *int | 否 | 仅允许 `0`（禁用）或 `1`（正常）；传 `2` 返回 `400 invalid status` |
| `nickname` | string | 否 | ≤64，为空串时保持原值 |

**响应（200）**：`data` 为更新后的用户信息。

**常见错误**：`404 user not found`、`400 invalid status`。

> 角色分配请使用 `PUT /api/v1/admin/users/:id/role`。

### DELETE `/api/v1/admin/users/:id`

删除用户（软删除）。

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：`404 user not found`。

### POST `/api/v1/admin/categories`

创建分类。

**请求体**

```json
{
  "name": "Go",
  "slug": "go",
  "description": "Go 语言相关",
  "sort": 0
}
```

| 字段 | 类型 | 必填 | 约束 |
| --- | --- | --- | --- |
| `name` | string | 是 | ≤64，全局唯一 |
| `slug` | string | 否 | ≤64，规则同文章 slug；缺省由 `name` 生成 |
| `description` | string | 否 | ≤255 |
| `sort` | int | 否 | 排序值，默认 0 |

**响应（201）**：`data` 为分类对象。

**常见错误**：`409 category name already exists`、`400 invalid slug`。

### PUT `/api/v1/admin/categories/:id`

更新分类。

**请求体**：同创建分类。

> `name` 与 `slug` 传入空串或与原值相同时不修改；`description`、`sort` 为覆盖写。

**响应（200）**：`data` 为更新后的分类对象。

**常见错误**：`404 category not found`、`409 category name already exists`、`400 invalid slug`。

### DELETE `/api/v1/admin/categories/:id`

删除分类。若仍有文章引用该分类则拒绝删除。

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：`404 category not found`、`409 category is in use by articles`。

### POST `/api/v1/admin/tags`

创建标签。

**请求体**

```json
{ "name": "并发", "slug": "concurrency" }
```

| 字段 | 类型 | 必填 | 约束 |
| --- | --- | --- | --- |
| `name` | string | 是 | ≤64，全局唯一 |
| `slug` | string | 否 | ≤64；缺省由 `name` 生成 |

**响应（201）**：`data` 为标签对象。

**常见错误**：`409 tag name already exists`、`400 invalid slug`。

### PUT `/api/v1/admin/tags/:id`

更新标签。

**请求体**：同创建标签（空串表示不修改）。

**响应（200）**：`data` 为更新后的标签对象。

**常见错误**：`404 tag not found`、`409 tag name already exists`、`400 invalid slug`。

### DELETE `/api/v1/admin/tags/:id`

删除标签（不校验引用，会同时解除文章关联）。

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：`404 tag not found`。

### GET `/api/v1/admin/articles`

管理员文章列表（全部状态）。

**查询参数**：`page`、`size`、`keyword`、`status`、`category_id`。

> 不支持 `tag_id` 过滤。排序同公开列表。

**响应（200）**：`data.list` 为文章摘要数组。

### PUT `/api/v1/admin/articles/:id/status`

更新文章状态。

> 该接口会推进文章的 `version`：状态属于 `ArticleRequest` 可覆盖的字段，推进版本才能拦住「作者用旧版本提交、把管理员刚归档的状态改回已发布」。

**请求体**

```json
{ "status": "published" }
```

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：`400 invalid request body`（状态非三种枚举之一）、`404 article not found`。

### PUT `/api/v1/admin/articles/:id/feature`

更新文章置顶/推荐标记。

**请求体**

```json
{
  "is_pinned": true,
  "is_featured": false
}
```

> 两个字段均为指针，未传入时保持原值；两者都未传返回 `400 nothing to update`。
> 该接口**不**推进 `version`：`is_pinned` / `is_featured` 不在 `ArticleRequest` 里，`PUT /articles/:id` 覆盖不到它们。

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：`400 nothing to update`、`404 article not found`。

### GET `/api/v1/admin/policies`

查询权限策略（`p`）与角色继承（`g`）。

**响应（200）**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "policies": [
      { "sub": "author", "obj": "/api/v1/articles", "act": "POST" }
    ],
    "roles": [
      { "child": "admin", "parent": "author" }
    ]
  }
}
```

> `obj` 支持 Casbin `keyMatch2` 通配（如 `/api/v1/admin/*`），`act` 支持正则（如 `(GET|POST|PUT|DELETE)`）。

### POST `/api/v1/admin/policies`

新增权限策略。

**请求体**

```json
{ "sub": "author", "obj": "/api/v1/articles/:id", "act": "GET" }
```

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：`409 policy already exists`。

### DELETE `/api/v1/admin/policies`

删除权限策略。

**请求体**

```json
{ "sub": "author", "obj": "/api/v1/articles/:id", "act": "GET" }
```

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：`404 policy not found`。

### POST `/api/v1/admin/roles`

新增角色继承（`child` 继承 `parent` 的权限）。

**请求体**

```json
{ "child": "author", "parent": "user" }
```

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：`409 role inheritance already exists`。

### DELETE `/api/v1/admin/roles`

删除角色继承。

**请求体**

```json
{ "child": "author", "parent": "user" }
```

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：`404 role inheritance not found`。

### PUT `/api/v1/admin/users/:id/role`

分配用户角色集合。

**请求体**

```json
{ "roles": ["admin", "author"] }
```

> `roles` 至少一项，取值限 `admin`/`author`/`user`（任一项非法即 `400 invalid request body`）；首个角色同时写入用户主 `role` 字段。

**响应（200）**

```json
{ "code": 0, "message": "success" }
```

**常见错误**：`404 user not found`、`400 roles must not be empty`。

### GET `/api/v1/admin/audits`

分页查询审计记录，按时间倒序。`keyword` 对操作者、动作、详情做模糊匹配。

**查询参数**：`page`、`size`、`keyword`。

**响应（200）**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [
      {
        "id": "uuid",
        "operator_id": "uuid",
        "operator_username": "admin",
        "action": "policy_add",
        "detail": "sub=author obj=/api/v1/articles/:id act=GET",
        "created_at": "2026-09-13T12:00:00+08:00"
      }
    ],
    "total": 1,
    "page": 1,
    "size": 10
  }
}
```

**审计动作（action）**

审计覆盖认证行为与管理端写操作；读接口与令牌刷新不记录。

| 分类 | action |
| --- | --- |
| 权限与角色 | `policy_add`、`policy_remove`、`role_add`、`role_remove`、`assign_role` |
| 认证与账户 | `register`、`login`、`login_failed`、`logout_all`、`delete_account`、`change_password`、`reset_password`、`verify_email` |
| 用户管理 | `user_update`、`user_delete` |
| 分类管理 | `category_create`、`category_update`、`category_delete` |
| 标签管理 | `tag_create`、`tag_update`、`tag_delete` |
| 文章管理 | `article_create`、`article_update`、`article_delete`、`article_status`、`article_feature` |
| 文件管理 | `file_upload`、`file_delete` |

> 审计为旁路能力：写入失败仅记录服务端日志，不影响原接口结果。`login_failed` 的 `operator_username` 为尝试登录的账号（账号不存在时 `operator_id` 为空串）。

### GET `/api/v1/admin/updater`

查询服务端自动更新的运行状态。状态由后台轮询与手动检查共同刷新，因此两次轮询之间可能滞后于远端最新 Release。

**响应（200）**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "enabled": true,
    "current_version": "v1.2.0",
    "update_available": true,
    "latest_version": "v1.3.0",
    "release_url": "https://github.com/owner/repo/releases/tag/v1.3.0",
    "source": "github",
    "last_checked_at": "2026-09-13T12:00:00+08:00"
  }
}
```

**字段**

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `enabled` | bool | 自动更新是否启用；`false` 时仅 `current_version` 有意义 |
| `current_version` | string | 当前二进制版本（构建时注入，非配置项） |
| `update_available` | bool | 是否发现更高版本 |
| `latest_version` | string | 可用新版本号，无可用更新时字段缺失 |
| `release_url` | string | 新版本 Release 页面地址，无可用更新时字段缺失 |
| `source` | string | 命中更新的来源：`github` 或 `gitee`，无可用更新时字段缺失 |
| `last_checked_at` | string | 最近一次检查时间，尚未检查过时字段缺失 |
| `last_error` | string | 最近一次检查或升级的错误信息，正常时字段缺失 |

> `last_error` 非空表示最近一次检查失败（与 `update_available` 独立），此时 `update_available` 仍保留上一次成功检查的结果。

### POST `/api/v1/admin/updater/check`

立即触发一次版本检查并刷新状态，**无需请求体**。检查为同步调用，受上游超时限制（每个来源最长 30 秒，多来源并发）。

**响应（200）**

响应体与 `GET /api/v1/admin/updater` 相同，为检查后的最新状态。

**说明**

- 本接口只做「发现」，不下载、不替换、不重启；升级由后台轮询在发现新版本后自动完成。
- 检查失败（来源不可用、网络异常等）仍返回 `200`，失败原因写入 `last_error`，前端据此提示。
- `enabled=false` 时返回 `409 auto update is disabled`。

---

## 订阅与站点文件

以下三个路径挂在**根路径**（不在 `/api` 下），无需鉴权、内容仅来自已发布文章。RSS 阅读器与搜索引擎爬虫按约定到根目录找这些文件，因此路径不可配置化到 `/api/v1`。

三者均返回 `Cache-Control: public, max-age=600`。

### GET `/feed.xml`

站点订阅源（RSS 2.0）。

- `Content-Type`：`application/rss+xml; charset=utf-8`。
- 输出最近 **20** 篇已发布文章，按公开列表排序。
- `channel.title` / `channel.description` / `channel.language` 取自 `site.*` 配置，`channel.link` 为 `site.base_url`。
- 每个 `item`：`link` 与 `guid` 均为 `{site.base_url}/articles/{slug}`，`description` 为文章**摘要**（正文是 Markdown 源文，服务端不渲染 HTML，因此不放进订阅源），`pubDate` 取 `published_at`（缺失时回退 `created_at`）。

### GET `/sitemap.xml`

站点地图。

- `Content-Type`：`application/xml; charset=utf-8`。
- 第一项固定为站点首页 `{site.base_url}/`，随后是已发布文章（最多 **1000** 篇），文章 `lastmod` 取 `updated_at`（`YYYY-MM-DD`）。

### GET `/robots.txt`

- `Content-Type`：`text/plain; charset=utf-8`。
- 内容固定为 `Allow: /` + `Disallow: /api/` + `Sitemap: {site.base_url}/sitemap.xml`。
- **不**屏蔽 `/uploads`：上传目录里是文章配图，屏蔽会让图片从搜索结果中消失。

> `site.base_url` 用于拼接上述绝对地址，配置需与前端实际访问域名一致，否则订阅源里的链接会指向错误主机。

---

## 静态资源

### GET `/uploads/:filename`

直接返回上传目录中的文件（无鉴权）。`filename` 为上传时生成的 `<uuid><ext>`，文件记录中的 `url` 字段可直接用于 `<img src>`。

---

## 数据模型与枚举

### 枚举

**用户角色（role / roles[]）**

| 值 | 说明 |
| --- | --- |
| `admin` | 管理员，继承 author 与 user |
| `author` | 作者，可管理文章与文件 |
| `user` | 普通用户，仅个人中心 |

**用户状态（status）**

| 值 | 说明 |
| --- | --- |
| 0 | 禁用（无法登录，且令牌立即失效） |
| 1 | 正常 |
| 2 | 未验证邮箱 |

**文章状态（status）**

| 值 | 说明 |
| --- | --- |
| `draft` | 草稿 |
| `published` | 已发布（公开可见） |
| `archived` | 已归档 |

### 用户信息对象（UserInfo）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | UUID |
| `username` | string | 用户名 |
| `email` | string | 邮箱 |
| `nickname` | string | 昵称 |
| `avatar` | string | 头像 URL |
| `bio` | string | 个人简介 |
| `role` | string | 主角色（`roles` 首个元素） |
| `roles` | []string \| null | 完整角色列表；**嵌套在文章 `author` 中时恒为 `null`** |
| `status` | int | 状态 |
| `created_at` | string | 创建时间 |

### 文章信息对象（ArticleInfo / ArticleSummary）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | UUID |
| `title` | string | 标题 |
| `slug` | string | 唯一别名 |
| `summary` | string | 摘要 |
| `content` | string | 正文（仅详情返回） |
| `cover_image` | string | 封面 URL |
| `status` | string | 状态 |
| `view_count` | int64 | 浏览量（按来源去重后累计） |
| `version` | int | 内容版本号（从 1 起）；**仅详情返回**，列表无此字段。编辑时原样回传以启用乐观锁 |
| `is_pinned` | bool | 是否置顶 |
| `is_featured` | bool | 是否推荐 |
| `category_id` | string | 分类 ID |
| `category` | object \| 缺省 | 分类对象 |
| `tags` | []object \| 缺省 | 标签对象数组 |
| `author_id` | string | 作者 ID |
| `author` | object \| 缺省 | 作者对象（`roles` 为 `null`） |
| `published_at` | string \| 缺省 | 首次发布时间（草稿无此字段） |
| `created_at` / `updated_at` | string | 创建/更新时间 |

### 文章修订对象（ArticleRevisionSummary / ArticleRevisionInfo）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 修订 ID（UUID） |
| `article_id` | string | 所属文章 ID |
| `editor_id` | string | 触发本次修订的操作者 ID |
| `version` | int | 版本号，同文章内自增（从 1 开始） |
| `title` | string | 该版本的标题 |
| `slug` | string | 该版本的 slug |
| `status` | string | 该版本的状态 |
| `created_at` | string | 修订写入时间 |
| `summary` | string | 该版本摘要（仅详情） |
| `content` | string | 该版本正文（仅详情） |
| `cover_image` | string | 该版本封面 URL（仅详情） |
| `category_id` | string | 该版本分类 ID（仅详情） |
| `tag_ids` | []string | 该版本标签 ID 数组（仅详情） |

> 修订保存的是**某次更新发生前的文章快照**（整份存储，非 diff），因此列表里版本号最大的那条 = 当前内容的上一版。
> 修订在文章创建时不产生，仅在 `PUT /api/v1/articles/:id` 与回滚接口中被写入。

### 分类（CategoryInfo）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | UUID |
| `name` | string | 名称 |
| `slug` | string | 唯一别名 |
| `description` | string | 描述 |
| `sort` | int | 排序值 |
| `created_at` | string | 创建时间 |

### 标签（TagInfo）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | UUID |
| `name` | string | 名称 |
| `slug` | string | 唯一别名 |
| `created_at` | string | 创建时间 |

### 文件（FileInfo）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | UUID |
| `name` | string | 原始文件名 |
| `url` | string | 访问路径，如 `/uploads/<uuid>.png` |
| `mime_type` | string | 上传时声明的 Content-Type |
| `size` | int64 | 字节数 |
| `uploader_id` | string | 上传者 ID |
| `created_at` | string | 创建时间 |

### 令牌对（TokenPair）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `access_token` | string | JWT |
| `access_expires_at` | string | access token 过期时间 |
| `refresh_token` | string | 随机字符串（仅此处返回明文） |
| `refresh_expires_at` | string | refresh token 过期时间 |

### 更新状态（UpdaterStatus）

见 [GET `/api/v1/admin/updater`](#get-apiv1adminupdater)。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `enabled` | bool | 自动更新是否启用 |
| `current_version` | string | 当前二进制版本 |
| `update_available` | bool | 是否发现更高版本 |
| `latest_version` | string | 可用新版本号（`omitempty`） |
| `release_url` | string | 新版本 Release 页面地址（`omitempty`） |
| `source` | string | 来源平台 `github`/`gitee`（`omitempty`） |
| `last_checked_at` | string | 最近一次检查时间（`omitempty`） |
| `last_error` | string | 最近一次检查或升级的错误（`omitempty`） |

---

## 前端对接注意事项

1. **令牌续期**：任一接口返回 `401`（`invalid or expired token`）时，用 `refresh_token` 调 `/auth/refresh`；因 refresh 为一次性轮换，并发请求需串行化刷新并共享新令牌，否则会因旧令牌已吊销而失败。
2. **`data` 可能不存在**：`code=0` 但无业务数据时（删除、登出类接口）响应体中没有 `data` 字段，不能直接解引用。
3. **字段可能不存在**：`category`、`tags`、`author`、`published_at` 为 `omitempty`，为空时字段缺失；`roles` 在嵌套作者对象中为 `null`。
4. **覆盖写字段**：`PUT /auth/profile` 的 `avatar`/`bio`、`PUT /articles/:id` 的多数内容字段、`PUT /admin/categories/:id` 的 `description`/`sort` 均为覆盖写，部分更新需先回传原值。
5. **权限判定建议**：前端仅依据 `user.roles` 做展示级隐藏，最终权限由服务端 Casbin 策略决定，需处理 `403`。
6. **限流与锁定**：登录失败 5 次将锁定 15 分钟，`429` 时不要高频重试。
7. **`400 invalid request body`** 不区分具体字段，前端需自行做前置校验并给出字段级提示。
8. **请求追踪**：可自行传入 `X-Request-Id`（非必填），服务端回显同一值；不传则生成 UUID。该头已加入 CORS 白名单与暴露头，浏览器端可自由设置并读取，报错时可一并反馈给后端定位。
9. **文章编辑需回传 `version`**：`GET /articles/:key`（公开详情）或 `GET /me/articles/:id`（作者详情）返回的 `version` 要随表单一起保存，`PUT /articles/:id` 时原样放进请求体。收到 `409` 表示文章已被他人（如管理员改状态）改动，此时应重新拉取详情、让用户决定保留哪一份，再重新提交——不要自动重试，否则等于把乐观锁关掉。
   > `version` 只在**详情**响应里，列表（`ArticleSummary`）没有；不传或传 `0` 时服务端不做并发检查。

---

## 升级注意事项

以下为近期实现变更，**已部署环境需按说明操作**，否则行为与本文档不一致：

1. **`POST /api/v1/auth/logout-all` 与 `DELETE /api/v1/auth/me` 已补充授权策略**：两条 `user` 策略已加入 `authz.SeedPolicies`。
   > `SeedPolicies` 现在会在每次启动时按条补齐缺失的种子策略，**已有环境重启服务即可自动生效**，无需清空 `casbin_rule` 表。
   > 代价是：用 `DELETE /api/v1/admin/policies` 删除的种子策略会在下次重启时被恢复。
2. **CORS 已允许并暴露 `X-Request-Id`**：若前端此前为规避预检失败而未发送该头，现在可以直接使用。
3. **`X-Request-Id` 增加格式约束**：仅接受 64 字符以内、由字母/数字/`-`/`_`/`.` 组成的值；不符合时服务端改用自己生成的 UUID，并在响应头回显实际使用的值。
4. **`jwt.secret` 不再有默认值**：缺失、为空或不足 16 字符时服务拒绝启动，升级前请确认配置文件里已设置强密钥。
5. **代理头默认不再被信任**：`server.trusted_proxies` 为空时忽略 `X-Forwarded-For`，限流按真实对端 IP 分桶；若服务前有反向代理（nginx 等），需在该项中显式填写代理 IP/CIDR，否则所有请求会被算作同一个来源。
6. **`POST /api/v1/auth/resend-verification` 不再返回 `404`**：邮箱未注册时与 `forgot-password` 一致地返回 `200`，以消除账号枚举信号。
7. **文章接口不再下发 `author.email`**：该字段在任何文章响应里都不再出现（`UserInfo.email` 加了 `omitempty`，作者信息构造时主动清空）。前端若曾读取该字段需改用 `/api/v1/admin/users`。
8. **邮箱验证/密码重置令牌改为只存哈希**：库中 `email_verification.token`、`password_reset.token` 现在是 SHA-256 哈希，明文只出现在邮件里。
   > 升级后**升级前已发出、尚未使用的验证/重置链接会全部失效**（提示 `invalid token`），用户重新申请一次即可；已经使用过的令牌不受影响。
9. **密钥类配置改由环境变量/`.env` 注入**：`config.yaml` 中 `database.password`、`jwt.secret`、`smtp.username/password`、`bootstrap.admin_password` 已留空。
   > 升级时必须先设置 `BLOG_DATABASE_PASSWORD`、`BLOG_JWT_SECRET` 等环境变量（或在与 `config.yaml` 同目录、exe 同目录放置 `.env`），否则：`jwt.secret` 缺失会**拒绝启动**；首个管理员尚不存在时 `BLOG_BOOTSTRAP_ADMIN_PASSWORD` 缺失也会拒绝启动。详见 [配置说明](配置说明.md)。
10. **文章检索语义变更**：`keyword` 现在先归一化再检索（切词、去重、最多 8 个 term 且全部命中），`%`、`_`、`+`、`-` 等字符不再具备通配/运算符含义。
    > 此前 `keyword=%` 会命中全部文章、`a-b` 会变成「含 a 且不含 b」。现在：有全文索引时前者返回空列表、后者按两个词的 AND 处理；无全文索引（`LIKE` 回退）时前者按字面 `%` 做子串匹配。前端若依赖旧行为需调整。
11. **新增 `article_revisions` 表与 `ft_article` 全文索引**：迁移随启动自动完成，无需手工建表；MySQL 需 8.0+，`ft_article` 创建失败（如账号无 `ALTER` 权限）只记录警告并降级为 `LIKE` 检索。
12. **新增根路径 `/feed.xml`、`/sitemap.xml`、`/robots.txt`**：无需鉴权，内容取自已发布文章。升级后请把 `site.base_url` 改为真实对外域名，否则订阅源与站点地图里的链接会指向 `http://localhost:8080`。
13. **文章新增乐观锁字段 `version`**：`articles` 表新增 `version` 列（迁移随启动自动完成，老数据被补为 `1`）。详情响应新增 `version`，`PUT /api/v1/articles/:id` 请求体新增可选 `version`。
    > **向后兼容**：不传或传 `0` 时不做并发检查，旧前端无需改动即可继续工作。要真正启用保护，前端需在编辑时回传该值（见[前端对接注意事项](#前端对接注意事项)第 9 条）。
    > 两个管理员/作者同时编辑同一篇文章时，后提交者现在会收到 `409` 而不是静默覆盖前者的改动。
14. **浏览量改为按来源去重**：`view_count` 不再每次请求都 +1，同一来源（IP + `User-Agent`）在窗口内重复打开同一篇文章只计一次，窗口由新增配置 `article.view_dedup_minutes` 控制（默认 30 分钟）。
    > 升级后 `view_count` 的增速会明显下降，这是**预期行为**（此前刷新页面、爬虫抓取都会累加），并非统计丢失。
    > 该去重状态保存在**进程内存**中：多实例部署时各实例各自计数（同一访客可能被多个实例各计一次），重启后窗口重置。若需要严格去重需引入外部存储，当前刻意不做以保持零外部依赖。

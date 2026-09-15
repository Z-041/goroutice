package authz

import (
	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	"gorm.io/gorm"
)

// modelText RBAC 权限模型：sub(角色) 对 obj(路径) 执行 act(HTTP 方法)。
const modelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && regexMatch(r.act, p.act)
`

// 种子策略中的角色与 HTTP 方法常量。
const (
	roleAdmin  = "admin"
	roleAuthor = "author"
	roleUser   = "user"
)

const (
	methodGet    = "GET"
	methodPost   = "POST"
	methodPut    = "PUT"
	methodDelete = "DELETE"
	methodAll    = "(GET|POST|PUT|DELETE)"
)

// NewEnforcer 基于 GORM 适配器创建 Casbin 执行器并加载已有策略。
func NewEnforcer(db *gorm.DB) (*casbin.Enforcer, error) {
	if err := db.AutoMigrate(&CasbinRule{}); err != nil {
		return nil, err
	}

	m, err := model.NewModelFromString(modelText)
	if err != nil {
		return nil, err
	}
	// NewEnforcer 内部会通过 adapter 自动加载已有策略。
	return casbin.NewEnforcer(m, newGormAdapter(db))
}

// SeedPolicies 初始化角色继承与权限策略。
// 逐条补齐而不是「策略表非空即跳过」：这些策略是接口的授权基线，若首次写入半途失败
// （或管理员误删了某条），跳过会让整片接口永久不可用且难以察觉；
// casbin 的 Add* 对已存在的规则天然幂等，重复调用不会产生重复数据。
func SeedPolicies(e *casbin.Enforcer) error {
	// 角色继承：admin 继承 author，author 继承 user。
	if _, err := e.AddGroupingPolicy(roleAdmin, roleAuthor); err != nil {
		return err
	}
	if _, err := e.AddGroupingPolicy(roleAuthor, roleUser); err != nil {
		return err
	}

	rules := [][]string{
		// user 及以上：个人中心
		{roleUser, "/api/v1/auth/profile", methodGet},
		{roleUser, "/api/v1/auth/profile", methodPut},
		{roleUser, "/api/v1/auth/password", methodPut},
		{roleUser, "/api/v1/auth/logout-all", methodPost},
		{roleUser, "/api/v1/auth/me", methodDelete},

		// author 及以上：文章与文件
		{roleAuthor, "/api/v1/articles", methodPost},
		{roleAuthor, "/api/v1/articles/:id", methodPut},
		{roleAuthor, "/api/v1/articles/:id", methodDelete},
		{roleAuthor, "/api/v1/me/articles", methodGet},
		{roleAuthor, "/api/v1/me/articles/:id", methodGet},
		// 修订历史：查看与回溯都只针对自己的文章，归属再由服务层按 author_id 二次校验。
		{roleAuthor, "/api/v1/me/articles/:id/revisions", methodGet},
		{roleAuthor, "/api/v1/me/articles/:id/revisions/:revisionId", methodGet},
		{roleAuthor, "/api/v1/me/articles/:id/revisions/:revisionId/restore", methodPost},
		{roleAuthor, "/api/v1/files", methodPost},
		{roleAuthor, "/api/v1/files", methodGet},
		{roleAuthor, "/api/v1/files/:id", methodDelete},

		// admin：管理端全部
		{roleAdmin, "/api/v1/admin/*", methodAll},
	}

	// AddGroupingPolicy / AddPolicies 会触发 Auto-Save 持久化，无需再显式 SavePolicy。
	if _, err := e.AddPolicies(rules); err != nil {
		return err
	}
	return nil
}

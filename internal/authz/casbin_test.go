package authz

import (
	"testing"

	"github.com/casbin/casbin/v3"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestEnforcer(t *testing.T) *casbin.Enforcer {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	// 内存数据库必须保持单连接，否则各连接间数据不可见。
	sqlDB.SetMaxOpenConns(1)

	e, err := NewEnforcer(db)
	if err != nil {
		t.Fatalf("new enforcer: %v", err)
	}
	if err := SeedPolicies(e); err != nil {
		t.Fatalf("seed policies: %v", err)
	}
	return e
}

func TestEnforcerRoles(t *testing.T) {
	e := newTestEnforcer(t)

	cases := []struct {
		name string
		sub  string
		obj  string
		act  string
		want bool
	}{
		{"admin list users", "admin", "/api/v1/admin/users", "GET", true},
		{"admin update article status", "admin", "/api/v1/admin/articles/abc/status", "PUT", true},
		{"author cannot access admin", "author", "/api/v1/admin/users", "GET", false},
		{"author create article", "author", "/api/v1/articles", "POST", true},
		{"author update article", "author", "/api/v1/articles/abc", "PUT", true},
		{"author upload file", "author", "/api/v1/files", "POST", true},
		{"user cannot create article", "user", "/api/v1/articles", "POST", false},
		{"user cannot upload file", "user", "/api/v1/files", "POST", false},
		{"user read profile", "user", "/api/v1/auth/profile", "GET", true},
		{"user change password", "user", "/api/v1/auth/password", "PUT", true},
		{"admin inherits author create article", "admin", "/api/v1/articles", "POST", true},
		{"author inherits user profile", "author", "/api/v1/auth/profile", "GET", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := e.Enforce(tc.sub, tc.obj, tc.act)
			if err != nil {
				t.Fatalf("enforce: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Enforce(%q,%q,%q) = %v, want %v", tc.sub, tc.obj, tc.act, got, tc.want)
			}
		})
	}
}

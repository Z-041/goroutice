package authz

import (
	"github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	"gorm.io/gorm"
)

// CasbinRule 存储 Casbin 策略规则，表名 casbin_rule。
type CasbinRule struct {
	ID    uint   `gorm:"primaryKey;autoIncrement"`
	PType string `gorm:"size:100"`
	V0    string `gorm:"size:100"`
	V1    string `gorm:"size:100"`
	V2    string `gorm:"size:100"`
	V3    string `gorm:"size:100"`
	V4    string `gorm:"size:100"`
	V5    string `gorm:"size:100"`
}

// TableName 指定表名。
func (CasbinRule) TableName() string { return "casbin_rule" }

// gormAdapter 基于 GORM 实现 Casbin 的 persist.Adapter，复用现有 MySQL 连接。
type gormAdapter struct {
	db *gorm.DB
}

func newGormAdapter(db *gorm.DB) *gormAdapter {
	return &gormAdapter{db: db}
}

// LoadPolicy 从数据库加载全部策略到模型。
func (a *gormAdapter) LoadPolicy(m model.Model) error {
	var rules []CasbinRule
	if err := a.db.Order("id").Find(&rules).Error; err != nil {
		return err
	}
	for _, r := range rules {
		if err := persist.LoadPolicyArray(buildLine(r), m); err != nil {
			return err
		}
	}
	return nil
}

// SavePolicy 清空并全量写入模型中的策略。
func (a *gormAdapter) SavePolicy(m model.Model) error {
	rules := collectRules(m)
	return a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM casbin_rule").Error; err != nil {
			return err
		}
		if len(rules) == 0 {
			return nil
		}
		return tx.Create(&rules).Error
	})
}

// AddPolicy 新增单条策略（Auto-Save）。
func (a *gormAdapter) AddPolicy(sec string, ptype string, rule []string) error {
	r := toRule(ptype, rule)
	return a.db.Create(&r).Error
}

// RemovePolicy 删除单条策略。
func (a *gormAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
	r := toRule(ptype, rule)
	return a.db.Where(
		"p_type = ? AND v0 = ? AND v1 = ? AND v2 = ? AND v3 = ? AND v4 = ? AND v5 = ?",
		r.PType, r.V0, r.V1, r.V2, r.V3, r.V4, r.V5,
	).Delete(&CasbinRule{}).Error
}

// AddPolicies 批量新增策略（BatchAdapter）。
func (a *gormAdapter) AddPolicies(sec string, ptype string, rules [][]string) error {
	records := make([]CasbinRule, 0, len(rules))
	for _, rule := range rules {
		records = append(records, toRule(ptype, rule))
	}
	if len(records) == 0 {
		return nil
	}
	return a.db.Create(&records).Error
}

// RemovePolicies 批量删除策略（BatchAdapter）。
func (a *gormAdapter) RemovePolicies(sec string, ptype string, rules [][]string) error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		for _, rule := range rules {
			r := toRule(ptype, rule)
			if err := tx.Where(
				"p_type = ? AND v0 = ? AND v1 = ? AND v2 = ? AND v3 = ? AND v4 = ? AND v5 = ?",
				r.PType, r.V0, r.V1, r.V2, r.V3, r.V4, r.V5,
			).Delete(&CasbinRule{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RemoveFilteredPolicy 按字段过滤删除策略。
func (a *gormAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	fields := []string{"v0", "v1", "v2", "v3", "v4", "v5"}
	tx := a.db.Where("p_type = ?", ptype)
	for i, v := range fieldValues {
		if idx := fieldIndex + i; idx >= 0 && idx < len(fields) {
			tx = tx.Where(fields[idx]+" = ?", v)
		}
	}
	return tx.Delete(&CasbinRule{}).Error
}

// buildLine 将记录转为 Casbin 策略行（首元素为 ptype）。
func buildLine(r CasbinRule) []string {
	line := []string{r.PType}
	for _, v := range []string{r.V0, r.V1, r.V2, r.V3, r.V4, r.V5} {
		if v != "" {
			line = append(line, v)
		}
	}
	return line
}

// toRule 将策略行转为 CasbinRule 记录。
func toRule(ptype string, rule []string) CasbinRule {
	r := CasbinRule{PType: ptype}
	vs := []*string{&r.V0, &r.V1, &r.V2, &r.V3, &r.V4, &r.V5}
	for i, v := range rule {
		if i < len(vs) {
			*vs[i] = v
		}
	}
	return r
}

// collectRules 遍历模型中的 p 与 g 策略并转为记录列表。
func collectRules(m model.Model) []CasbinRule {
	var rules []CasbinRule
	for ptype, ast := range m["p"] {
		for _, rule := range ast.Policy {
			rules = append(rules, toRule(ptype, rule))
		}
	}
	for ptype, ast := range m["g"] {
		for _, rule := range ast.Policy {
			rules = append(rules, toRule(ptype, rule))
		}
	}
	return rules
}

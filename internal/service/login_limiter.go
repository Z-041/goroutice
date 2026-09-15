package service

import (
	"context"
	"strings"
	"sync"
	"time"
)

// normalizeLoginKey 归一化登录失败计数键。
// 用户名与邮箱在 MySQL 的默认排序规则下大小写不敏感（`admin` 与 `Admin` 命中同一行），
// 若按原样字符串计数，攻击者只要轮换大小写就能每次落到新的计数桶，锁定形同虚设。
func normalizeLoginKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

type loginEntry struct {
	failures    int
	lockedUntil time.Time
	lastSeen    time.Time
}

// LoginLimiter 基于账号的内存登录失败锁定器，用于防暴力破解。
type LoginLimiter struct {
	mu          sync.Mutex
	entries     map[string]*loginEntry
	maxAttempts int
	lockWindow  time.Duration
}

// NewLoginLimiter 构造登录失败锁定器。
func NewLoginLimiter(maxAttempts, lockMinutes int) *LoginLimiter {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if lockMinutes <= 0 {
		lockMinutes = 15
	}
	return &LoginLimiter{
		entries:     make(map[string]*loginEntry),
		maxAttempts: maxAttempts,
		lockWindow:  time.Duration(lockMinutes) * time.Minute,
	}
}

// Locked 判断账号当前是否处于锁定期。
func (l *LoginLimiter) Locked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	key = normalizeLoginKey(key)
	e, ok := l.entries[key]
	if !ok {
		return false
	}
	now := time.Now()
	e.lastSeen = now

	if e.lockedUntil.IsZero() {
		return false
	}
	if now.Before(e.lockedUntil) {
		return true
	}
	// 锁定期已过，重置该账号状态。
	delete(l.entries, key)
	return false
}

// Fail 记录一次失败，返回是否因此触发锁定。
func (l *LoginLimiter) Fail(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	key = normalizeLoginKey(key)
	now := time.Now()
	e, ok := l.entries[key]
	if !ok {
		e = &loginEntry{}
		l.entries[key] = e
	}
	e.lastSeen = now
	e.failures++
	if e.failures >= l.maxAttempts {
		e.lockedUntil = now.Add(l.lockWindow)
		e.failures = 0
		return true
	}
	return false
}

// Success 登录成功后清除失败记录。
func (l *LoginLimiter) Success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, normalizeLoginKey(key))
}

// Cleanup 周期性清理超过 idleTimeout 未活动的记录，防止内存无限增长。
// 当 ctx 取消时退出。
func (l *LoginLimiter) Cleanup(ctx context.Context, idleTimeout time.Duration) {
	ticker := time.NewTicker(idleTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			l.mu.Lock()
			for key, e := range l.entries {
				// 仍在锁定期的条目必须保留：清理周期（1 分钟）远小于锁定时长（默认 15 分钟），
				// 一并删除会让攻击者只需静默一个周期就重新获得完整的尝试次数。
				if e.lockedUntil.After(now) {
					continue
				}
				if now.Sub(e.lastSeen) > idleTimeout {
					delete(l.entries, key)
				}
			}
			l.mu.Unlock()
		}
	}
}

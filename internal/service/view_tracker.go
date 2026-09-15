package service

import (
	"context"
	"sync"
	"time"
)

// defaultViewDedupMinutes 是浏览量去重窗口的默认值（分钟）。
const defaultViewDedupMinutes = 30

// ViewTracker 基于「来源 + 文章」的内存浏览去重器。
//
// 浏览量原本是每次请求都 +1，刷新页面、爬虫抓取、作者自己调试全都算数，
// 跑一段时间后这个数字就不再代表「有多少人读过」，前端却仍把它当人气指标展示。
// 这里给同一来源对同一篇文章的重复访问设一个时间窗口，窗口内只计一次。
//
// 去重键包含 User-Agent：只用 IP 的话，同一出口 IP 下的所有读者（公司内网、运营商 NAT）
// 会被当成同一个人，热门文章的浏览量反而被系统性压低。
// 代价是刷量者换个 UA 就能绕过——去重只用来让数字可信，不作为安全边界。
type ViewTracker struct {
	mu     sync.Mutex
	seen   map[string]time.Time
	window time.Duration
	// now 用于注入时钟：测试替换它即可控制时间流逝，不必真的 sleep 出分钟级窗口。
	now func() time.Time
}

// NewViewTracker 构造浏览去重器，windowMinutes <= 0 时使用默认窗口。
func NewViewTracker(windowMinutes int) *ViewTracker {
	if windowMinutes <= 0 {
		windowMinutes = defaultViewDedupMinutes
	}
	return &ViewTracker{
		seen:   make(map[string]time.Time),
		window: time.Duration(windowMinutes) * time.Minute,
		now:    time.Now,
	}
}

// Fresh 判断本次访问是否应计入浏览量：该来源在窗口内已读过这篇文章时返回 false。
func (t *ViewTracker) Fresh(articleID, viewer string) bool {
	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()

	key := articleID + "|" + viewer
	if last, ok := t.seen[key]; ok && now.Sub(last) < t.window {
		return false
	}
	t.seen[key] = now
	return true
}

// Cleanup 周期性清理已过窗口的记录，避免内存随访客数无限增长；ctx 取消时退出。
// period 是扫描周期，与被清理记录的年龄无关。
//
// 这里不能按 period 删除记录：period 通常是一分钟，而窗口默认 30 分钟，
// 按 period 删除等于把窗口压缩到一分钟，同一访客刷新两次就会被重复计数，去重形同虚设。
func (t *ViewTracker) Cleanup(ctx context.Context, period time.Duration) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.sweep()
		}
	}
}

// sweep 删除已过窗口的记录并返回删除条数。
func (t *ViewTracker) sweep() int {
	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()

	removed := 0
	for key, last := range t.seen {
		if now.Sub(last) >= t.window {
			delete(t.seen, key)
			removed++
		}
	}
	return removed
}

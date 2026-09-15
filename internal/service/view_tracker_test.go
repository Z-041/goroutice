package service

import (
	"context"
	"testing"
	"time"
)

// newTestViewTracker 返回时钟可控的去重器，以及一个推进时钟的函数。
// 窗口是分钟级的，真 sleep 出 30 分钟显然不现实，所以直接替换内部时钟。
func newTestViewTracker(windowMinutes int) (*ViewTracker, func(time.Duration)) {
	tr := NewViewTracker(windowMinutes)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return now }
	return tr, func(d time.Duration) { now = now.Add(d) }
}

func TestViewTracker_DedupWithinWindow(t *testing.T) {
	tr, advance := newTestViewTracker(30)

	if !tr.Fresh("article-1", "10.0.0.1|agent") {
		t.Fatal("首次访问应计入")
	}
	advance(29 * time.Minute)
	if tr.Fresh("article-1", "10.0.0.1|agent") {
		t.Fatal("窗口内同一来源重复访问不应计入")
	}
	if !tr.Fresh("article-2", "10.0.0.1|agent") {
		t.Fatal("不同文章之间不应互相去重")
	}

	// 距首次访问已 31 分钟，超过 30 分钟窗口。
	advance(2 * time.Minute)
	if !tr.Fresh("article-1", "10.0.0.1|agent") {
		t.Fatal("窗口过期后应重新计入")
	}
}

func TestViewTracker_ZeroWindowFallsBackToDefault(t *testing.T) {
	tr := NewViewTracker(0)
	if tr.window != defaultViewDedupMinutes*time.Minute {
		t.Fatalf("window = %v，期望回退到默认值 %d 分钟", tr.window, defaultViewDedupMinutes)
	}
}

// TestViewTracker_SweepKeepsRecordsInWindow 守住一个容易踩的坑：
// 清理周期（分钟级）远小于去重窗口（默认 30 分钟），按周期删除记录等于把窗口压缩到一分钟。
func TestViewTracker_SweepKeepsRecordsInWindow(t *testing.T) {
	tr, advance := newTestViewTracker(30)

	tr.Fresh("article-1", "a")
	advance(10 * time.Minute)
	tr.Fresh("article-2", "b")

	if removed := tr.sweep(); removed != 0 {
		t.Fatalf("sweep 删除了 %d 条仍在窗口内的记录，去重窗口会被压缩", removed)
	}
	if tr.Fresh("article-1", "a") {
		t.Fatal("sweep 后 article-1 的记录应仍在，重复访问不应被计入")
	}

	// 再过 21 分钟：article-1 已 31 分钟（过期），article-2 仅 21 分钟（仍有效）。
	advance(21 * time.Minute)
	if removed := tr.sweep(); removed != 1 {
		t.Fatalf("sweep 删除了 %d 条记录，期望只删掉已过窗口的那 1 条", removed)
	}
	if tr.Fresh("article-2", "b") {
		t.Fatal("article-2 的记录应被保留，重复访问不应被计入")
	}
}

func TestViewTracker_CleanupExitsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	tr := NewViewTracker(30)
	go func() {
		tr.Cleanup(ctx, time.Millisecond)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Cleanup 未在 ctx 取消后退出")
	}
}

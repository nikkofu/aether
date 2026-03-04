package gateway

import (
	"sync"
	"time"
)

// RateLimiter 实现了基于内存的简单速率控制。
type RateLimiter struct {
	mu           sync.Mutex
	skillCounts  map[string]int // skillID -> count
	orgCounts    map[string]int // orgID -> count
	lastReset    time.Time
	SkillLimit   int
	OrgLimit     int
}

func NewRateLimiter(skillLimit, orgLimit int) *RateLimiter {
	return &RateLimiter{
		skillCounts: make(map[string]int),
		orgCounts:   make(map[string]int),
		lastReset:   time.Now(),
		SkillLimit:  skillLimit,
		OrgLimit:    orgLimit,
	}
}

// Allow 检查当前请求是否触发限流。
func (l *RateLimiter) Allow(skillID, orgID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 每分钟重置一次计数器
	if time.Since(l.lastReset) >= time.Minute {
		l.skillCounts = make(map[string]int)
		l.orgCounts = make(map[string]int)
		l.lastReset = time.Now()
	}

	// 1. 检查 Skill 级限流
	if l.skillCounts[skillID] >= l.SkillLimit {
		return false
	}

	// 2. 检查 Org 级限流
	if l.orgCounts[orgID] >= l.OrgLimit {
		return false
	}

	// 3. 更新计数
	l.skillCounts[skillID]++
	l.orgCounts[orgID]++

	return true
}

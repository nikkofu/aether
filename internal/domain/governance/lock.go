package governance

import (
	"sync"
)

// GovernanceLock 提供系统级的手动接管模式控制。
type GovernanceLock struct {
	mu         sync.RWMutex
	manualMode bool
}

func NewGovernanceLock() *GovernanceLock {
	return &GovernanceLock{}
}

func (l *GovernanceLock) EnableManualMode() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.manualMode = true
}

func (l *GovernanceLock) DisableManualMode() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.manualMode = false
}

func (l *GovernanceLock) IsManualMode() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.manualMode
}

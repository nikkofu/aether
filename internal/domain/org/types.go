package org

import (
	"github.com/nikkofu/aether/internal/domain/agent"
)

// OrgLevel 定义了代理在组织架构中的层级。
type OrgLevel string

const (
	LevelVision      OrgLevel = "vision"
	LevelStrategic   OrgLevel = "strategic"
	LevelTactical    OrgLevel = "tactical"
	LevelOperational OrgLevel = "operational"
	LevelGovernance  OrgLevel = "governance"
)

// OrgAgent 是具备组织拓扑属性的代理接口。
type OrgAgent interface {
	agent.Agent // 继承基础代理接口

	ID() string
	Level() OrgLevel
	Supervisor() string
	Subordinates() []string
}

// OrgRegistry 管理组织拓扑。
type OrgRegistry interface {
	Register(a OrgAgent)
	Get(id string) (OrgAgent, bool)
	GetByLevel(level OrgLevel) []OrgAgent
	Hierarchy() map[string][]string
}

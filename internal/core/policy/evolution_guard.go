package policy

// EvolutionGuard 负责系统级进化行为的准入控制。
type EvolutionGuard struct {
	allowedModules map[string]bool
}

func NewEvolutionGuard() *EvolutionGuard {
	return &EvolutionGuard{
		allowedModules: map[string]bool{
			"skills":   true,
			"strategy": true,
		},
	}
}

// AllowEvolution 检查指定模块是否允许执行自主演进逻辑。
func (g *EvolutionGuard) AllowEvolution(module string) bool {
	allowed, ok := g.allowedModules[module]
	return ok && allowed
}

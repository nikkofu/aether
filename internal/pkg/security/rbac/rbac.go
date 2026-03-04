package rbac

import (
	"sync"
)

type DefaultRBAC struct {
	mu          sync.RWMutex
	assignments map[string]map[string]Role // orgID -> userID -> Role
	permissions map[Role][]Permission
}

func NewDefaultRBAC() *DefaultRBAC {
	r := &DefaultRBAC{
		assignments: make(map[string]map[string]Role),
		permissions: make(map[Role][]Permission),
	}
	r.setupDefaultMatrix()
	return r
}

func (r *DefaultRBAC) setupDefaultMatrix() {
	r.permissions[RoleAdmin] = []Permission{PermCreateProposal, PermVoteProposal, PermModifyEconomy, PermViewAudit, PermSpawnAgent}
	r.permissions[RoleOperator] = []Permission{PermCreateProposal, PermVoteProposal, PermSpawnAgent}
	r.permissions[RoleViewer] = []Permission{PermViewAudit}
	r.permissions[RoleAgent] = []Permission{PermVoteProposal}
}

func (r *DefaultRBAC) AssignRole(userID string, role Role, orgID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.assignments[orgID] == nil {
		r.assignments[orgID] = make(map[string]Role)
	}
	r.assignments[orgID][userID] = role
	return nil
}

func (r *DefaultRBAC) CheckPermission(userID string, perm Permission, orgID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.assignments[orgID] == nil { return false }
	role, ok := r.assignments[orgID][userID]
	if !ok { return false }
	for _, p := range r.permissions[role] {
		if p == perm { return true }
	}
	return false
}

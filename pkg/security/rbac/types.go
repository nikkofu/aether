package rbac

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
	RoleAgent    Role = "agent"
)

type Permission string

const (
	PermCreateProposal Permission = "create_proposal"
	PermVoteProposal   Permission = "vote_proposal"
	PermModifyEconomy  Permission = "modify_economy"
	PermViewAudit      Permission = "view_audit"
	PermSpawnAgent     Permission = "spawn_agent"
)

type RBAC interface {
	AssignRole(userID string, role Role, orgID string) error
	CheckPermission(userID string, perm Permission, orgID string) bool
}

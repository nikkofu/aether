package governance

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nikkofu/aether/internal/pkg/audit"
	"github.com/nikkofu/aether/internal/core/economy"
	"github.com/nikkofu/aether/internal/core/governance/constitution"
	"github.com/nikkofu/aether/internal/pkg/logging"
	"github.com/nikkofu/aether/internal/pkg/security/rbac"
)

// PolicyProposal 代表系统的一个治理提案。
type PolicyProposal struct {
	ID                     string          `json:"id"`
	OrgID                  string          `json:"org_id"`
	CreatorID              string          `json:"creator_id"`
	Title                  string          `json:"title"`
	Proposal               string          `json:"proposal"`
	PolicyType             string          `json:"policy_type"`
	NewValue               any             `json:"new_value"`
	RequiresVisionApproval bool            `json:"requires_vision_approval"`
	VisionApproved         bool            `json:"vision_approved"`
	Votes                  map[string]bool `json:"votes"`
	CreatedAt              time.Time       `json:"created_at"`
	Status                 string          `json:"status"`
}

// GovernanceBoard 负责处理提案的统计、权限校验与锁定检查。
type GovernanceBoard struct {
	mu           sync.RWMutex
	ledger       economy.Ledger
	constitution constitution.Constitution
	rbac         rbac.RBAC
	audit        audit.Logger
	lock         *GovernanceLock
	logger       logging.Logger
	proposals    map[string]*PolicyProposal
}

func NewGovernanceBoard(l economy.Ledger, c constitution.Constitution, r rbac.RBAC, a audit.Logger, lock *GovernanceLock, log logging.Logger) *GovernanceBoard {
	return &GovernanceBoard{
		ledger:       l,
		constitution: c,
		rbac:         r,
		audit:        a,
		lock:         lock,
		logger:       log,
		proposals:    make(map[string]*PolicyProposal),
	}
}

func (b *GovernanceBoard) SubmitProposal(ctx context.Context, p *PolicyProposal) error {
	if b.lock != nil && b.lock.IsManualMode() {
		return fmt.Errorf("系统处于人工接管模式，禁止自动提交提案")
	}

	if !b.rbac.CheckPermission(p.CreatorID, rbac.PermCreateProposal, p.OrgID) {
		b.audit.Log(ctx, p.OrgID, audit.EventConstRejected, "无权限创建提案", map[string]any{"user": p.CreatorID})
		return fmt.Errorf("unauthorized to create proposal")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if p.CreatedAt.IsZero() { p.CreatedAt = time.Now() }
	if p.Votes == nil { p.Votes = make(map[string]bool) }
	p.Status = "pending"
	b.proposals[p.ID] = p
	
	b.audit.Log(ctx, p.OrgID, audit.EventProposalCreated, p.Title, map[string]any{"id": p.ID, "creator": p.CreatorID})
	return nil
}

func (b *GovernanceBoard) Vote(ctx context.Context, proposalID, agentID string, approve bool) error {
	b.mu.RLock()
	p, ok := b.proposals[proposalID]
	b.mu.RUnlock()
	if !ok { return fmt.Errorf("proposal not found") }

	if !b.rbac.CheckPermission(agentID, rbac.PermVoteProposal, p.OrgID) {
		return fmt.Errorf("unauthorized to vote")
	}

	b.mu.Lock()
	p.Votes[agentID] = approve
	b.mu.Unlock()
	
	b.audit.Log(ctx, p.OrgID, audit.EventProposalVoted, "提案投票", map[string]any{"proposal": proposalID, "voter": agentID, "approve": approve})
	return nil
}

func (b *GovernanceBoard) ApproveByVision(proposalID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.proposals[proposalID]
	if !ok { return fmt.Errorf("proposal not found") }
	p.VisionApproved = true
	return nil
}

func (b *GovernanceBoard) Tally(ctx context.Context, proposalID string) (bool, error) {
	b.mu.Lock()
	p, ok := b.proposals[proposalID]
	b.mu.Unlock()
	if !ok { return false, fmt.Errorf("proposal not found") }

	if p.RequiresVisionApproval && !p.VisionApproved {
		return false, fmt.Errorf("requires vision layer approval")
	}

	var totalWeight, approvedWeight float64
	for agentID, approve := range p.Votes {
		acc, err := b.ledger.GetAccount(ctx, p.OrgID, agentID)
		if err != nil { continue }
		weight := acc.Reputation
		if weight <= 0 { weight = 1.0 }
		totalWeight += weight
		if approve { approvedWeight += weight }
	}

	if totalWeight == 0 { return false, nil }
	passed := (approvedWeight / totalWeight) > 0.51

	if !passed {
		b.mu.Lock()
		p.Status = "rejected"
		b.mu.Unlock()
		return false, nil
	}

	if b.constitution != nil {
		if err := b.constitution.ValidatePolicyChange(p.PolicyType, p.NewValue); err != nil {
			b.mu.Lock()
			p.Status = "unconstitutional"
			b.mu.Unlock()
			b.audit.Log(ctx, p.OrgID, audit.EventConstRejected, "提案违宪", map[string]any{"id": p.ID, "err": err.Error()})
			return false, err
		}
	}

	b.mu.Lock()
	p.Status = "passed"
	b.mu.Unlock()
	b.audit.Log(ctx, p.OrgID, audit.EventProposalPassed, "提案通过", map[string]any{"id": p.ID})

	return true, nil
}

func (b *GovernanceBoard) ListProposals(orgID string) []*PolicyProposal {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var res []*PolicyProposal
	for _, p := range b.proposals {
		if orgID == "" || p.OrgID == orgID { res = append(res, p) }
	}
	return res
}

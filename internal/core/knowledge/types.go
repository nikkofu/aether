package knowledge

import (
	"context"
	"time"
)

type Entity struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Name      string         `json:"name"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type Relation struct {
	ID        string         `json:"id"`
	FromID    string         `json:"from_id"`
	ToID      string         `json:"to_id"`
	Type      string         `json:"type"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type Graph interface {
	AddEntity(ctx context.Context, e Entity, orgID string) error
	AddRelation(ctx context.Context, r Relation, orgID string) error
	GetEntity(ctx context.Context, id string) (*Entity, error) // ID 通常全局唯一
	GetRelations(ctx context.Context, orgID string, id string) ([]Relation, error)
	QueryByType(ctx context.Context, orgID string, entityType string) ([]Entity, error)
}

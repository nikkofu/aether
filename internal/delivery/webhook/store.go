package webhook

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

const (
	DeliveryStatusPending  = "pending"
	DeliveryStatusAccepted = "accepted"
	DeliveryStatusIgnored  = "ignored"
	DeliveryStatusFailed   = "failed"
)

type DeliveryRecord struct {
	DeliveryID    string    `json:"delivery_id"`
	Event         string    `json:"event"`
	PayloadSHA256 string    `json:"payload_sha256"`
	Status        string    `json:"status"`
	TaskID        string    `json:"task_id,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type DeliveryStore interface {
	Acquire(ctx context.Context, input DeliveryRecord) (*DeliveryRecord, bool, error)
	Complete(ctx context.Context, deliveryID, status, taskID, errorMessage string) error
}

type SQLiteDeliveryStore struct {
	db *sql.DB
}

func NewSQLiteDeliveryStore(db *sql.DB) (*SQLiteDeliveryStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	store := &SQLiteDeliveryStore{db: db}
	if err := store.init(context.Background()); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *SQLiteDeliveryStore) init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS webhook_deliveries (
		delivery_id TEXT PRIMARY KEY,
		event TEXT NOT NULL,
		payload_sha256 TEXT NOT NULL,
		status TEXT NOT NULL,
		task_id TEXT,
		error_message TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status ON webhook_deliveries(status);
	CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_updated_at ON webhook_deliveries(updated_at DESC);
	`

	_, err := s.db.ExecContext(ctx, query)
	return err
}

func (s *SQLiteDeliveryStore) Acquire(ctx context.Context, input DeliveryRecord) (*DeliveryRecord, bool, error) {
	now := time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	record, err := s.getTx(ctx, tx, input.DeliveryID)
	if err == sql.ErrNoRows {
		record = &DeliveryRecord{
			DeliveryID:    input.DeliveryID,
			Event:         input.Event,
			PayloadSHA256: input.PayloadSHA256,
			Status:        DeliveryStatusPending,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO webhook_deliveries (delivery_id, event, payload_sha256, status, task_id, error_message, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			record.DeliveryID,
			record.Event,
			record.PayloadSHA256,
			record.Status,
			record.TaskID,
			record.ErrorMessage,
			record.CreatedAt,
			record.UpdatedAt,
		); err != nil {
			return nil, false, err
		}

		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		tx = nil
		return record, true, nil
	}
	if err != nil {
		return nil, false, err
	}

	if record.PayloadSHA256 != input.PayloadSHA256 {
		return nil, false, fmt.Errorf("delivery payload hash mismatch for %s", input.DeliveryID)
	}

	if record.Status == DeliveryStatusFailed {
		record.Status = DeliveryStatusPending
		record.TaskID = ""
		record.ErrorMessage = ""
		record.UpdatedAt = now

		if _, err := tx.ExecContext(
			ctx,
			`UPDATE webhook_deliveries
			 SET status = ?, task_id = ?, error_message = ?, updated_at = ?
			 WHERE delivery_id = ?`,
			record.Status,
			record.TaskID,
			record.ErrorMessage,
			record.UpdatedAt,
			record.DeliveryID,
		); err != nil {
			return nil, false, err
		}

		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		tx = nil
		return record, true, nil
	}

	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	tx = nil
	return record, false, nil
}

func (s *SQLiteDeliveryStore) Complete(ctx context.Context, deliveryID, status, taskID, errorMessage string) error {
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE webhook_deliveries
		 SET status = ?, task_id = ?, error_message = ?, updated_at = ?
		 WHERE delivery_id = ?`,
		status,
		taskID,
		errorMessage,
		time.Now().UTC(),
		deliveryID,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("delivery not found: %s", deliveryID)
	}
	return nil
}

func (s *SQLiteDeliveryStore) getTx(ctx context.Context, tx *sql.Tx, deliveryID string) (*DeliveryRecord, error) {
	row := tx.QueryRowContext(
		ctx,
		`SELECT delivery_id, event, payload_sha256, status, COALESCE(task_id, ''), COALESCE(error_message, ''), created_at, updated_at
		 FROM webhook_deliveries WHERE delivery_id = ?`,
		deliveryID,
	)

	record := &DeliveryRecord{}
	if err := row.Scan(
		&record.DeliveryID,
		&record.Event,
		&record.PayloadSHA256,
		&record.Status,
		&record.TaskID,
		&record.ErrorMessage,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return record, nil
}

type InMemoryDeliveryStore struct {
	mu         sync.Mutex
	deliveries map[string]*DeliveryRecord
}

func NewInMemoryDeliveryStore() *InMemoryDeliveryStore {
	return &InMemoryDeliveryStore{
		deliveries: make(map[string]*DeliveryRecord),
	}
}

func (s *InMemoryDeliveryStore) Acquire(ctx context.Context, input DeliveryRecord) (*DeliveryRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	record, ok := s.deliveries[input.DeliveryID]
	if !ok {
		record = &DeliveryRecord{
			DeliveryID:    input.DeliveryID,
			Event:         input.Event,
			PayloadSHA256: input.PayloadSHA256,
			Status:        DeliveryStatusPending,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		s.deliveries[input.DeliveryID] = record
		return cloneDeliveryRecord(record), true, nil
	}

	if record.PayloadSHA256 != input.PayloadSHA256 {
		return nil, false, fmt.Errorf("delivery payload hash mismatch for %s", input.DeliveryID)
	}

	if record.Status == DeliveryStatusFailed {
		record.Status = DeliveryStatusPending
		record.TaskID = ""
		record.ErrorMessage = ""
		record.UpdatedAt = now
		return cloneDeliveryRecord(record), true, nil
	}

	return cloneDeliveryRecord(record), false, nil
}

func (s *InMemoryDeliveryStore) Complete(ctx context.Context, deliveryID, status, taskID, errorMessage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.deliveries[deliveryID]
	if !ok {
		return fmt.Errorf("delivery not found: %s", deliveryID)
	}

	record.Status = status
	record.TaskID = taskID
	record.ErrorMessage = errorMessage
	record.UpdatedAt = time.Now().UTC()
	return nil
}

func cloneDeliveryRecord(record *DeliveryRecord) *DeliveryRecord {
	if record == nil {
		return nil
	}
	cloned := *record
	return &cloned
}

var _ DeliveryStore = (*SQLiteDeliveryStore)(nil)
var _ DeliveryStore = (*InMemoryDeliveryStore)(nil)

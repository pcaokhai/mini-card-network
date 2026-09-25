// Package rotation drives the GENERATE -> SEND_0800_161 -> PARTNER_CONFIRM -> ACTIVATE key
// rotation workflow (MCN-504) and persists it in key_rotation/audit_log.
package rotation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mcn/gateway-go/internal/store"
)

// Step names, fixed by contracts/openapi.yaml's KeyRotation.steps[].name enum (Ruling 1 -
// MCN-504-ISS builds against this exact vocabulary).
const (
	StepGenerate       = "GENERATE"
	StepSend0800161    = "SEND_0800_161"
	StepPartnerConfirm = "PARTNER_CONFIRM"
	StepActivate       = "ACTIVATE"

	StatusPending = "PENDING"
	StatusDone    = "DONE"
	StatusFailed  = "FAILED"
)

// ErrNotFound means no key_rotation row has the requested id.
var ErrNotFound = errors.New("rotation not found")

var stepOrder = []string{StepGenerate, StepSend0800161, StepPartnerConfirm, StepActivate}

// Step mirrors contracts/openapi.yaml's KeyRotation.steps[] entry.
type Step struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// Row is one key_rotation row.
type Row struct {
	ID        int64
	KeyType   string
	Status    string
	Steps     []Step
	NewKCV    *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Repository persists key_rotation and audit_log.
type Repository struct{ pool *store.Pool }

// NewRepository builds a Repository backed by pool.
func NewRepository(pool *store.Pool) *Repository { return &Repository{pool: pool} }

// Create inserts a RUNNING rotation with all four steps PENDING and returns its id.
func (r *Repository) Create(ctx context.Context, keyType string) (int64, error) {
	steps := make([]Step, len(stepOrder))
	for i, name := range stepOrder {
		steps[i] = Step{Name: name, Status: StatusPending}
	}
	raw, err := json.Marshal(steps)
	if err != nil {
		return 0, fmt.Errorf("marshal steps: %w", err)
	}
	var id int64
	err = r.pool.QueryRow(ctx,
		`INSERT INTO key_rotation (key_type, status, steps) VALUES ($1, 'RUNNING', $2) RETURNING id`,
		keyType, raw,
	).Scan(&id)
	return id, err
}

// UpdateStep flips the named step's status (and completedAt, when DONE) in one row-level update.
func (r *Repository) UpdateStep(ctx context.Context, id int64, stepName, stepStatus string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT steps FROM key_rotation WHERE id = $1 FOR UPDATE`, id).Scan(&raw); err != nil {
		return fmt.Errorf("select steps: %w", err)
	}
	var steps []Step
	if err := json.Unmarshal(raw, &steps); err != nil {
		return fmt.Errorf("unmarshal steps: %w", err)
	}
	now := time.Now().UTC()
	for i := range steps {
		if steps[i].Name != stepName {
			continue
		}
		steps[i].Status = stepStatus
		if stepStatus == StatusDone {
			steps[i].CompletedAt = &now
		}
	}
	updated, err := json.Marshal(steps)
	if err != nil {
		return fmt.Errorf("marshal steps: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE key_rotation SET steps = $1, updated_at = now() WHERE id = $2`, updated, id); err != nil {
		return fmt.Errorf("update steps: %w", err)
	}
	return tx.Commit(ctx)
}

// Complete marks the rotation COMPLETED with the newly activated key's KCV.
func (r *Repository) Complete(ctx context.Context, id int64, newKCV string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE key_rotation SET status = 'COMPLETED', new_kcv = $1, updated_at = now() WHERE id = $2`,
		newKCV, id)
	return err
}

// Fail marks the rotation FAILED.
func (r *Repository) Fail(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE key_rotation SET status = 'FAILED', updated_at = now() WHERE id = $1`, id)
	return err
}

// FailAllRunning marks every RUNNING rotation FAILED, flagging each still-PENDING step FAILED.
// Only the runner calls it, at startup, when a RUNNING row can only be one a crash interrupted.
func (r *Repository) FailAllRunning(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE key_rotation SET status = 'FAILED', updated_at = now(),
		   steps = (SELECT jsonb_agg(CASE WHEN s->>'status' = 'PENDING' THEN jsonb_set(s, '{status}', '"FAILED"') ELSE s END)
		            FROM jsonb_array_elements(steps) s)
		 WHERE status = 'RUNNING'`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Get loads one key_rotation row.
func (r *Repository) Get(ctx context.Context, id int64) (Row, error) {
	var row Row
	var raw []byte
	err := r.pool.QueryRow(ctx,
		`SELECT id, key_type, status, steps, new_kcv, created_at, updated_at FROM key_rotation WHERE id = $1`, id,
	).Scan(&row.ID, &row.KeyType, &row.Status, &raw, &row.NewKCV, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Row{}, ErrNotFound
	}
	if err != nil {
		return Row{}, err
	}
	if err := json.Unmarshal(raw, &row.Steps); err != nil {
		return Row{}, fmt.Errorf("unmarshal steps: %w", err)
	}
	return row, nil
}

// WriteAudit appends one append-only audit_log record.
func (r *Repository) WriteAudit(ctx context.Context, eventType string, detail any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal audit detail: %w", err)
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO audit_log (event_type, detail) VALUES ($1, $2)`, eventType, raw)
	return err
}

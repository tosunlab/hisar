package store

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// AuditEvent is one row of audit_events; AccountEmail is nil when the account is gone or the event has none.
type AuditEvent struct {
	ID           int64
	AccountID    *string
	AccountEmail *string
	DeviceID     *string
	Type         string
	Detail       []byte
	IP           string
	CreatedAt    time.Time
}

// AuditFilter narrows the audit log; a zero field matches every event.
type AuditFilter struct {
	// AccountID matches the events of one account, including a deleted one.
	AccountID string
	// AccountEmail matches the events of the account that has this email now.
	AccountEmail string
	Types        []string
	From, To     time.Time
}

// AuditCursor is the last event of a page: the next page starts with the event just older than it.
type AuditCursor struct {
	CreatedAt time.Time
	ID        int64
}

func (filter AuditFilter) buildQuery(after *AuditCursor) (string, []any) {
	var conditions []string
	var arguments []any
	addCondition := func(condition string, values ...any) {
		for _, value := range values {
			arguments = append(arguments, value)
			condition = strings.Replace(condition, "$?", "$"+strconv.Itoa(len(arguments)), 1)
		}
		conditions = append(conditions, condition)
	}
	if filter.AccountID != "" {
		addCondition(`e.account_id = $?`, filter.AccountID)
	}
	if filter.AccountEmail != "" {
		addCondition(`e.account_id = (SELECT id FROM accounts WHERE email = $?)`, filter.AccountEmail)
	}
	if len(filter.Types) > 0 {
		addCondition(`e.type = ANY($?)`, filter.Types)
	}
	if !filter.From.IsZero() {
		addCondition(`e.created_at >= $?`, filter.From)
	}
	if !filter.To.IsZero() {
		addCondition(`e.created_at < $?`, filter.To)
	}
	if after != nil {
		addCondition(`(e.created_at, e.id) < ($?, $?)`, after.CreatedAt, after.ID)
	}
	query := `SELECT e.id, e.account_id, a.email, e.device_id::text, e.type, e.detail, coalesce(e.ip, ''), e.created_at
		FROM audit_events e LEFT JOIN accounts a ON a.id = e.account_id`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	return query + " ORDER BY e.created_at DESC, e.id DESC", arguments
}

func scanAuditEvent(row pgx.CollectableRow) (AuditEvent, error) {
	var event AuditEvent
	err := row.Scan(&event.ID, &event.AccountID, &event.AccountEmail, &event.DeviceID, &event.Type, &event.Detail, &event.IP, &event.CreatedAt)
	return event, err
}

// ListAuditEvents returns up to limit events that match filter, newest first, starting after the cursor when there is
// one.
func (s *Store) ListAuditEvents(ctx context.Context, filter AuditFilter, after *AuditCursor, limit int) ([]AuditEvent, error) {
	query, arguments := filter.buildQuery(after)
	arguments = append(arguments, limit)
	rows, err := s.pool.Query(ctx, query+" LIMIT $"+strconv.Itoa(len(arguments)), arguments...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanAuditEvent)
}

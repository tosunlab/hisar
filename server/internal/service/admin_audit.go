package service

import (
	"cmp"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/store"
)

// auditExportBatchSize is how many events an export reads at a time; the database connection is free between batches.
const auditExportBatchSize = 500

var errInvalidAuditCursor = newError(KindInvalid, "This page of the audit log is no longer valid. Load the audit log again.")

var auditLabelsByName = func() map[string]string {
	labels := make(map[string]string, len(auditTypes))
	for _, row := range auditTypes {
		labels[row.name] = row.label
	}
	return labels
}()

// AuditQuery is the audit log's filters as the admin's request names them; an empty field matches every event.
type AuditQuery struct {
	// Account is an account's email or ID; an ID also finds a deleted account's events.
	Account     string
	Type, Group string
	// From and To are RFC 3339 times; From is inclusive and To exclusive.
	From, To string
	Cursor   string
}

// GetAuditTypes lists the audit log's types in groups, for the dashboard's filter.
func (s *Service) GetAuditTypes() apitypes.AuditTypesResponse {
	groups := make([]apitypes.AuditTypeGroup, 0, len(auditGroups))
	for _, group := range auditGroups {
		types := []apitypes.AuditType{}
		for _, row := range auditTypes {
			if row.group == group.id {
				types = append(types, apitypes.AuditType{Type: row.name, Label: row.label})
			}
		}
		groups = append(groups, apitypes.AuditTypeGroup{ID: group.id, Label: group.label, Types: types})
	}
	return apitypes.AuditTypesResponse{Groups: groups}
}

// LabelAuditType is the dashboard's label of a stored type name; a name this server does not know stays as it is.
func LabelAuditType(name string) string {
	if label, ok := auditLabelsByName[name]; ok {
		return label
	}
	return name
}

// ListAuditEvents returns one page of the audit log, newest first.
func (s *Service) ListAuditEvents(ctx context.Context, query AuditQuery) (apitypes.AuditPage, error) {
	filter, err := convertAuditQuery(query)
	if err != nil {
		return apitypes.AuditPage{}, err
	}
	after, err := parseAuditCursor(query.Cursor)
	if err != nil {
		return apitypes.AuditPage{}, err
	}
	events, err := s.store.ListAuditEvents(ctx, filter, after, apitypes.AuditPageSize+1)
	if err != nil {
		return apitypes.AuditPage{}, err
	}
	page := apitypes.AuditPage{Events: convertAuditEvents(events[:min(len(events), apitypes.AuditPageSize)])}
	if len(events) > apitypes.AuditPageSize {
		last := events[apitypes.AuditPageSize-1]
		page.NextCursor = strconv.FormatInt(last.CreatedAt.UnixMicro(), 10) + "." + strconv.FormatInt(last.ID, 10)
	}
	return page, nil
}

// AuditExport reads the events of an export that StartAuditExport checked and recorded.
type AuditExport struct {
	store  *store.Store
	filter store.AuditFilter
	after  *store.AuditCursor
	isDone bool
}

// StartAuditExport checks the filters of an export and records it as admin_audit_exported with them.
func (s *Service) StartAuditExport(ctx context.Context, actor Actor, query AuditQuery) (*AuditExport, error) {
	filter, err := convertAuditQuery(query)
	if err != nil {
		return nil, err
	}
	detail := map[string]string{}
	for name, value := range map[string]string{"account": cmp.Or(filter.AccountEmail, filter.AccountID), "type": query.Type, "group": query.Group, "from": query.From, "to": query.To} {
		if value != "" {
			detail[name] = value
		}
	}
	s.recordAudit(ctx, actor, auditAdminAuditExported, detail)
	return &AuditExport{store: s.store, filter: filter}, nil
}

// Next returns the export's next events, newest first, and no events after the last ones.
func (export *AuditExport) Next(ctx context.Context) ([]apitypes.AuditEvent, error) {
	if export.isDone {
		return nil, nil
	}
	events, err := export.store.ListAuditEvents(ctx, export.filter, export.after, auditExportBatchSize)
	if err != nil {
		return nil, err
	}
	if len(events) < auditExportBatchSize {
		export.isDone = true
	} else {
		last := events[len(events)-1]
		export.after = &store.AuditCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return convertAuditEvents(events), nil
}

func convertAuditQuery(query AuditQuery) (store.AuditFilter, error) {
	var filter store.AuditFilter
	if account := strings.TrimSpace(query.Account); strings.Contains(account, "@") {
		filter.AccountEmail = apitypes.NormalizeEmail(account)
	} else {
		filter.AccountID = account
	}
	if query.Type != "" && query.Group != "" {
		return store.AuditFilter{}, newError(KindInvalid, "Filter by one type or one group, not both.")
	}
	if query.Type != "" {
		if _, ok := auditLabelsByName[query.Type]; !ok {
			return store.AuditFilter{}, newError(KindInvalid, "Unknown audit event type "+strconv.Quote(query.Type)+".")
		}
		filter.Types = []string{query.Type}
	}
	if query.Group != "" {
		for _, row := range auditTypes {
			if row.group == query.Group {
				filter.Types = append(filter.Types, row.name)
			}
		}
		if len(filter.Types) == 0 {
			return store.AuditFilter{}, newError(KindInvalid, "Unknown group of audit event types "+strconv.Quote(query.Group)+".")
		}
	}
	var err error
	if filter.From, err = parseAuditTime("from", query.From); err != nil {
		return store.AuditFilter{}, err
	}
	if filter.To, err = parseAuditTime("to", query.To); err != nil {
		return store.AuditFilter{}, err
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && !filter.From.Before(filter.To) {
		return store.AuditFilter{}, newError(KindInvalid, "The start of the date range must be before its end.")
	}
	return filter, nil
}

func parseAuditTime(name, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, newError(KindInvalid, strconv.Quote(name)+" must be a time like 2026-10-18T09:00:00Z.")
	}
	return parsed, nil
}

// lastAuditCursorTime bounds a cursor's time to the years a filter accepts, which Postgres stores; a time outside
// them would fail the query instead of answering errInvalidAuditCursor.
var lastAuditCursorTime = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC).UnixMicro()

func parseAuditCursor(cursor string) (*store.AuditCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	microseconds, id, isCut := strings.Cut(cursor, ".")
	createdAt, timeErr := strconv.ParseInt(microseconds, 10, 64)
	eventID, idErr := strconv.ParseInt(id, 10, 64)
	if !isCut || timeErr != nil || idErr != nil || createdAt < 0 || createdAt >= lastAuditCursorTime {
		return nil, errInvalidAuditCursor
	}
	return &store.AuditCursor{CreatedAt: time.UnixMicro(createdAt), ID: eventID}, nil
}

func convertAuditEvents(events []store.AuditEvent) []apitypes.AuditEvent {
	converted := make([]apitypes.AuditEvent, 0, len(events))
	for _, event := range events {
		converted = append(converted, apitypes.AuditEvent{
			ID: event.ID, AccountID: event.AccountID, AccountEmail: event.AccountEmail, DeviceID: event.DeviceID, Type: event.Type,
			Detail: event.Detail, IP: event.IP, CreatedAt: event.CreatedAt,
		})
	}
	return converted
}

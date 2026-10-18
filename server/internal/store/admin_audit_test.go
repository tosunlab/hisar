package store

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestAdminAuditPagesThroughEventsAtTheSameTime(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	older := insertAuditEvent(t, database, "AAAAAA", "login", base)
	tiedFirst := insertAuditEvent(t, database, "AAAAAA", "login", base.Add(time.Minute))
	tiedSecond := insertAuditEvent(t, database, "AAAAAA", "reauthentication_failed", base.Add(time.Minute))
	newest := insertAuditEvent(t, database, "AAAAAA", "login", base.Add(2*time.Minute))

	var listed []int64
	var cursor *AuditCursor
	for range 10 {
		page, err := database.ListAuditEvents(ctx, AuditFilter{}, cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		listed = append(listed, page[0].ID)
		cursor = &AuditCursor{CreatedAt: page[0].CreatedAt, ID: page[0].ID}
	}
	if expected := []int64{newest, tiedSecond, tiedFirst, older}; !slices.Equal(listed, expected) {
		t.Errorf("pages %v, expected %v", listed, expected)
	}
}

func TestAdminAuditFilters(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	aliceLogin := insertAuditEvent(t, database, "AAAAAA", "login", base)
	aliceFailure := insertAuditEvent(t, database, "AAAAAA", "reauthentication_failed", base.Add(time.Hour))
	bobLogin := insertAuditEvent(t, database, "BBBBBB", "login", base.Add(2*time.Hour))
	bobVault := insertAuditEvent(t, database, "BBBBBB", "vault_created", base.Add(3*time.Hour))

	for name, test := range map[string]struct {
		filter   AuditFilter
		expected []int64
	}{
		"none":            {AuditFilter{}, []int64{bobVault, bobLogin, aliceFailure, aliceLogin}},
		"account ID":      {AuditFilter{AccountID: "AAAAAA"}, []int64{aliceFailure, aliceLogin}},
		"account email":   {AuditFilter{AccountEmail: "b@example.com"}, []int64{bobVault, bobLogin}},
		"unknown email":   {AuditFilter{AccountEmail: "nobody@example.com"}, nil},
		"type":            {AuditFilter{Types: []string{"login"}}, []int64{bobLogin, aliceLogin}},
		"types":           {AuditFilter{Types: []string{"reauthentication_failed", "vault_created"}}, []int64{bobVault, aliceFailure}},
		"from":            {AuditFilter{From: base.Add(2 * time.Hour)}, []int64{bobVault, bobLogin}},
		"to":              {AuditFilter{To: base.Add(time.Hour)}, []int64{aliceLogin}},
		"range":           {AuditFilter{From: base.Add(time.Hour), To: base.Add(3 * time.Hour)}, []int64{bobLogin, aliceFailure}},
		"combined":        {AuditFilter{AccountEmail: "b@example.com", Types: []string{"login"}, From: base, To: base.Add(3 * time.Hour)}, []int64{bobLogin}},
		"combined, empty": {AuditFilter{AccountID: "AAAAAA", Types: []string{"vault_created"}}, nil},
	} {
		events, err := database.ListAuditEvents(ctx, test.filter, nil, 100)
		if err != nil {
			t.Fatal(err)
		}
		if listed := listAuditEventIDs(events); !slices.Equal(listed, test.expected) {
			t.Errorf("%s: %v, expected %v", name, listed, test.expected)
		}
	}
}

func TestAdminAuditKeepsADeletedAccountsEvents(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	login := insertAuditEvent(t, database, "AAAAAA", "login", time.Now())
	events, err := database.ListAuditEvents(ctx, AuditFilter{AccountID: "AAAAAA"}, nil, 100)
	if err != nil || len(events) != 1 || events[0].AccountEmail == nil || *events[0].AccountEmail != "a@example.com" {
		t.Fatalf("before deletion: %+v %v", events, err)
	}
	if _, err := database.pool.Exec(ctx, `DELETE FROM accounts WHERE id = 'AAAAAA'`); err != nil {
		t.Fatal(err)
	}

	events, err = database.ListAuditEvents(ctx, AuditFilter{AccountID: "AAAAAA"}, nil, 100)
	if err != nil || len(events) != 1 || events[0].ID != login || events[0].AccountID == nil || *events[0].AccountID != "AAAAAA" || events[0].AccountEmail != nil {
		t.Errorf("after deletion: %+v %v", events, err)
	}
	if events, err := database.ListAuditEvents(ctx, AuditFilter{AccountEmail: "a@example.com"}, nil, 100); err != nil || len(events) != 0 {
		t.Errorf("by the former email: %+v %v", events, err)
	}
}

func insertAuditEvent(t *testing.T, database *Store, accountID, eventType string, createdAt time.Time) int64 {
	t.Helper()
	var id int64
	if err := database.pool.QueryRow(context.Background(),
		`INSERT INTO audit_events (account_id, type, detail, ip, created_at) VALUES ($1, $2, '{"device": "Mac"}', '192.0.2.1', $3) RETURNING id`,
		accountID, eventType, createdAt).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func listAuditEventIDs(events []AuditEvent) []int64 {
	var ids []int64
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}

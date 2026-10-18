package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
)

func TestAdminAuditLog(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpTestAccount(t, ctx, server, baseURL, "member@example.com", apitypes.RoleMember)
	get := func(token, path string, query url.Values) *http.Response {
		t.Helper()
		request := requireValue[*http.Request](t)(http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/admin/"+path+"?"+query.Encode(), nil))
		request.Header.Set("Authorization", "Bearer "+token)
		return requireValue[*http.Response](t)(http.DefaultClient.Do(request))
	}
	listPage := func(query url.Values) apitypes.AuditPage {
		t.Helper()
		response := get(admin.Client.Token, "audit", query)
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("audit %v: %d", query, response.StatusCode)
		}
		var page apitypes.AuditPage
		if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
			t.Fatal(err)
		}
		return page
	}

	for _, path := range []string{"audit", "audit/types", "audit.csv"} {
		response := get(member.Client.Token, path, nil)
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Errorf("member %s: %d", path, response.StatusCode)
		}
	}

	page := listPage(url.Values{"type": {"signup"}})
	if len(page.Events) != 2 || *page.Events[0].AccountEmail != "member@example.com" || *page.Events[1].AccountEmail != "admin@example.com" || page.NextCursor != "" {
		t.Fatalf("sign-ups: %+v", page)
	}
	for account, accountID := range map[string]string{"Member@Example.com": member.AccountID, admin.AccountID: admin.AccountID} {
		events := listPage(url.Values{"account": {account}}).Events
		eventCount := queryValue[int](t, databaseURL, `SELECT count(*) FROM audit_events WHERE account_id = $1`, accountID)
		if len(events) != eventCount || eventCount < 2 {
			t.Errorf("%s: %d events, %d in the database", account, len(events), eventCount)
		}
		for _, event := range events {
			if *event.AccountID != accountID {
				t.Errorf("%s: %+v", account, event)
			}
		}
	}
	for _, query := range []url.Values{
		{"type": {"nonsense"}}, {"group": {"nonsense"}}, {"type": {"login"}, "group": {"accounts"}}, {"from": {"yesterday"}},
		{"cursor": {"x"}}, {"cursor": {"-300000000000000000.1"}}, {"cursor": {"300000000000000000.1"}},
	} {
		response := get(admin.Client.Token, "audit", query)
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("%v: %d", query, response.StatusCode)
		}
	}

	connection := requireValue[*pgx.Conn](t)(pgx.Connect(ctx, databaseURL))
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, `INSERT INTO audit_events (account_id, type, detail, ip, created_at)
		SELECT $1, 'reauthentication_failed', '{"note": "a, \"quoted\" value\nover two lines"}', '192.0.2.1', now() - make_interval(secs => n % 7) FROM generate_series(1, 1050) n`,
		member.AccountID); err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	query := url.Values{"type": {"reauthentication_failed"}}
	for pageCount := 1; ; pageCount++ {
		page := listPage(query)
		for _, event := range page.Events {
			if seen[event.ID] || event.Type != "reauthentication_failed" {
				t.Fatalf("page %d repeats or mismatches %+v", pageCount, event)
			}
			seen[event.ID] = true
		}
		if page.NextCursor == "" {
			if pageCount != 11 || len(page.Events) != 50 {
				t.Errorf("last page %d holds %d events", pageCount, len(page.Events))
			}
			break
		}
		query.Set("cursor", page.NextCursor)
	}
	if len(seen) != 1050 {
		t.Errorf("paged through %d events", len(seen))
	}

	from := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	response := get(admin.Client.Token, "audit.csv", url.Values{"account": {member.AccountID}, "group": {"accounts"}, "from": {from}})
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Disposition"), `attachment; filename="hisar-audit-`) ||
		response.Header.Get("Content-Type") != "text/csv; charset=utf-8" {
		t.Fatalf("export: %d %v", response.StatusCode, response.Header)
	}
	records := requireValue[[][]string](t)(csv.NewReader(response.Body).ReadAll())
	if len(records) != 1052 || strings.Join(records[0], ",") != "ID,Time,Event,Type,Account ID,Account email,Device ID,IP address,Details" {
		t.Fatalf("export holds %d records, header %v", len(records), records[0])
	}
	countByType := map[string]int{}
	for _, record := range records[1:] {
		countByType[record[3]]++
		if record[3] == "reauthentication_failed" && (record[2] != "Master password confirmation failed" || record[4] != member.AccountID || record[5] != "member@example.com" ||
			record[8] != `{"note": "a, \"quoted\" value\nover two lines"}`) {
			t.Fatalf("exported row %q", record)
		}
	}
	if countByType["reauthentication_failed"] != 1050 || countByType["signup"] != 1 {
		t.Errorf("exported types %v", countByType)
	}
	exportDetail := queryValue[string](t, databaseURL, `SELECT detail::text FROM audit_events WHERE type = 'admin_audit_exported' AND account_id = $1`, admin.AccountID)
	var recordedFilters map[string]string
	if err := json.Unmarshal([]byte(exportDetail), &recordedFilters); err != nil || len(recordedFilters) != 3 ||
		recordedFilters["account"] != member.AccountID || recordedFilters["group"] != "accounts" || recordedFilters["from"] != from {
		t.Errorf("recorded export %s %v", exportDetail, err)
	}
}

func TestAdminAuditExportOfADeletedAccount(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "member@example.com", apitypes.RoleMember, nil))
	password := []byte("member password")
	member, secretKey, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "member@example.com", "Member", password, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if err := member.DeleteAccount(ctx, password, secretKey, "member@example.com"); err != nil {
		t.Fatal(err)
	}
	request := requireValue[*http.Request](t)(http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/admin/audit.csv?account="+member.AccountID, nil))
	request.Header.Set("Authorization", "Bearer "+admin.Client.Token)
	response := requireValue[*http.Response](t)(http.DefaultClient.Do(request))
	defer response.Body.Close()
	body := string(requireValue[[]byte](t)(io.ReadAll(response.Body)))
	records := requireValue[[][]string](t)(csv.NewReader(strings.NewReader(body)).ReadAll())
	if len(records) != 4 || records[1][3] != "account_deleted" || records[3][3] != "signup" {
		t.Fatalf("export %q", body)
	}
	for _, record := range records[1:] {
		if record[4] != member.AccountID || record[5] != "" {
			t.Errorf("deleted account's row %q", record)
		}
	}
	if count := queryValue[int](t, databaseURL, `SELECT count(*) FROM audit_events WHERE account_id = $1`, member.AccountID); count != 3 {
		t.Errorf("%d events of the deleted account", count)
	}
}

func TestFormatAuditRowEscapesFormulas(t *testing.T) {
	email, accountID := "=HYPERLINK(\"x\")@example.com", "AAAAAA"
	row := formatAuditRow(apitypes.AuditEvent{ID: 7, AccountID: &accountID, AccountEmail: &email, Type: "unknown_type", IP: "-1", CreatedAt: time.Unix(0, 0)})
	expected := []string{"7", "1970-01-01T00:00:00Z", "unknown_type", "unknown_type", "AAAAAA", "'=HYPERLINK(\"x\")@example.com", "", "'-1", ""}
	if strings.Join(row, "|") != strings.Join(expected, "|") {
		t.Errorf("row %q", row)
	}
}

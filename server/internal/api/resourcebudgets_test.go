package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

// lastAnswer keeps the status and body of the latest response, as any client, old or new, receives them.
type lastAnswer struct {
	status int
	body   []byte
}

func (answer *lastAnswer) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		return nil, err
	}
	answer.status, answer.body = response.StatusCode, body
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}

func TestHistoryBudgetRefusalIsNotAnItemConflict(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "history@example.com", apitypes.RoleMember, nil))
	answer := &lastAnswer{}
	client := apiclient.New(baseURL)
	client.HTTP.Transport = answer
	alice, _, err := session.SignUp(ctx, client, invite.Token, "history@example.com", "Alice", []byte("public fixture password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	defer alice.Keyset.Wipe()
	vaults := requireValue[[]session.Vault](t)(alice.ListVaults(ctx))
	note := &item.Item{Overview: item.Overview{Title: "note", Category: item.CategorySecureNote}, Details: item.Details{Notes: "x"}}
	if _, err := alice.SaveItem(ctx, &vaults[0], note); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, `INSERT INTO item_history(vault_id,id,revision,deleted,overview,details,updated_at,updated_by)
	 SELECT vault_id,id,gen_random_uuid(),false,'x',repeat('x',1048576)::bytea,now(),updated_by FROM items,generate_series(1,1024)`); err != nil {
		t.Fatal(err)
	}

	note.Overview.Title = "edited at the history budget"
	current, err := alice.SaveItem(ctx, &vaults[0], note)
	if errors.Is(err, session.ErrConflict) || current != nil {
		t.Fatalf("the history budget reached the user as an item conflict: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "item history") || strings.Contains(err.Error(), "server:") {
		t.Fatalf("the user did not get the history budget sentence: %v", err)
	}
	if answer.status == http.StatusConflict || answer.status < 400 || !strings.Contains(string(answer.body), `"error":"This vault's item history`) {
		t.Fatalf("an older client cannot read the refusal: %d %s", answer.status, answer.body)
	}
}

func TestSignInLoopPastTheDeviceBudget(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "devices@example.com", apitypes.RoleMember, nil))
	password := []byte("public fixture password")
	alice, secretKey, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "devices@example.com", "Alice", password, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	defer alice.Keyset.Wipe()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, `
		WITH old AS (INSERT INTO devices(id,account_id,name,platform) SELECT gen_random_uuid(),$1,'old','go' FROM generate_series(1,255) RETURNING id)
		INSERT INTO sessions(token_hash,account_id,device_id,expires_at) SELECT uuid_send(gen_random_uuid()),$1,id,now()-interval '1 day' FROM old WHERE random()<0.5`,
		alice.AccountID); err != nil {
		t.Fatal(err)
	}

	for attempt := range 4 {
		signedIn, err := session.LogIn(ctx, apiclient.New(baseURL), alice.Email, password, secretKey, testDevice)
		if err != nil {
			t.Fatalf("sign-in %d with every other device signed out: %v", attempt, err)
		}
		signedIn.Keyset.Wipe()
		if err := signedIn.Client.Logout(ctx); err != nil {
			t.Fatal(err)
		}
	}
	devices := requireValue[[]apitypes.Device](t)(alice.Client.ListDevices(ctx))
	if len(devices) != 2 {
		t.Fatalf("Devices lists %d devices, want the sign-up device and the last signed-out one", len(devices))
	}

	if _, err := connection.Exec(ctx, `
		WITH live AS (INSERT INTO devices(id,account_id,name,platform) SELECT gen_random_uuid(),$1,'live','go' FROM generate_series(1,254) RETURNING id)
		INSERT INTO sessions(token_hash,account_id,device_id,expires_at) SELECT uuid_send(gen_random_uuid()),$1,id,now()+interval '1 day' FROM live`,
		alice.AccountID); err != nil {
		t.Fatal(err)
	}
	lastSignedIn, err := session.LogIn(ctx, apiclient.New(baseURL), alice.Email, password, secretKey, testDevice)
	if err != nil {
		t.Fatalf("the 256th signed-in device was refused while a signed-out one held its place: %v", err)
	}
	lastSignedIn.Keyset.Wipe()
	_, err = session.LogIn(ctx, apiclient.New(baseURL), alice.Email, password, secretKey, testDevice)
	if err == nil || !strings.Contains(err.Error(), "Sign out devices you no longer use") || strings.Contains(err.Error(), "server:") {
		t.Fatalf("a sign-in past 256 signed-in devices did not name the fix: %v", err)
	}
	if count := len(requireValue[[]apitypes.Device](t)(alice.Client.ListDevices(ctx))); count != 256 {
		t.Fatalf("a signed-in device was signed out to make room: %d devices", count)
	}
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/sharelink"
)

type fakeShareLinkAPI struct {
	mutex          sync.Mutex
	linksByID      map[string]apitypes.ShareLink
	ciphertexts    map[string]apitypes.PublicShareLink
	createdCounter int
}

// serveShareLinkAPI answers the share link endpoints and passes everything else to the other test API.
func serveShareLinkAPI(t *testing.T, testService unlockedTestService) *fakeShareLinkAPI {
	t.Helper()
	api := &fakeShareLinkAPI{linksByID: map[string]apitypes.ShareLink{}, ciphertexts: map[string]apitypes.PublicShareLink{}}
	service := testService.service
	service.mutex.Lock()
	upstream, err := url.Parse(service.unlockedSession.Client.BaseURL)
	service.mutex.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.Contains(request.URL.Path, "/share-links") {
			proxy.ServeHTTP(writer, request)
			return
		}
		api.answer(writer, request)
	}))
	t.Cleanup(httpServer.Close)
	service.mutex.Lock()
	client := *service.unlockedSession.Client
	client.BaseURL = httpServer.URL
	service.unlockedSession.Client = &client
	service.mutex.Unlock()
	return api
}

func (api *fakeShareLinkAPI) answer(writer http.ResponseWriter, request *http.Request) {
	api.mutex.Lock()
	defer api.mutex.Unlock()
	writer.Header().Set("Content-Type", "application/json")
	id := request.URL.Path[strings.LastIndex(request.URL.Path, "/")+1:]
	switch {
	case request.Method == http.MethodPost:
		var create apitypes.CreateShareLinkRequest
		json.NewDecoder(request.Body).Decode(&create)
		api.createdCounter++
		id = strings.Repeat(string(rune('A'+api.createdCounter)), 22)
		link := apitypes.ShareLink{ID: id, EncryptedLabel: create.EncryptedLabel, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Duration(create.ExpiresInSeconds) * time.Second), MaxViews: create.MaxViews, HasPassphrase: create.Passphrase != nil}
		api.linksByID[id] = link
		api.ciphertexts[id] = apitypes.PublicShareLink{Ciphertext: create.Ciphertext, Passphrase: create.Passphrase, ExpiresAt: link.ExpiresAt, MaxViews: link.MaxViews}
		writer.WriteHeader(http.StatusCreated)
		json.NewEncoder(writer).Encode(link)
	case request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/public/"):
		public, isFound := api.ciphertexts[id]
		if !isFound {
			writer.WriteHeader(http.StatusNotFound)
			writer.Write([]byte(`{"error":"not found"}`))
			return
		}
		json.NewEncoder(writer).Encode(public)
	case request.Method == http.MethodGet:
		links := []apitypes.ShareLink{}
		for _, link := range api.linksByID {
			links = append(links, link)
		}
		json.NewEncoder(writer).Encode(links)
	case request.Method == http.MethodDelete:
		if _, isFound := api.linksByID[id]; !isFound {
			writer.WriteHeader(http.StatusNotFound)
			writer.Write([]byte(`{"error":"not found"}`))
			return
		}
		delete(api.linksByID, id)
		delete(api.ciphertexts, id)
		writer.WriteHeader(http.StatusNoContent)
	}
}

func TestCommandLineShareLinksThroughTheApp(t *testing.T) {
	test := createCommandLineTest(t)
	service := test.testService.service
	api := serveShareLinkAPI(t, test.testService)
	if _, err := service.SaveItem(test.testService.personalID, ItemDetail{
		Title: "GitHub", Username: "alice", Password: "github-password", Fields: []ItemField{{Name: "Recovery code", Value: "a1b2", Concealed: true}}, OneTimePassword: testOneTimePassword,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(test.testService.personalID, ItemDetail{Title: "GitLab", Username: "alice", Password: "gitlab-password"}); err != nil {
		t.Fatal(err)
	}

	create := cliprotocol.Request{Type: cliprotocol.TypeCreateShareLink, Vault: "personal", Query: "github",
		ShareLink: &cliprotocol.ShareLinkChange{Fields: []string{"password", "recovery code"}, ExpiresInSeconds: 86400, MaxViews: 2}}
	var created cliprotocol.ShareLinkResult
	test.mustSend(t, create, &created)
	if created.ShareLink.Title != "GitHub" || strings.Join(created.ShareLink.FieldLabels, ",") != "password,Recovery code" || created.ShareLink.MaxViews != 2 {
		t.Fatalf("created %+v", created.ShareLink)
	}
	fetched, err := sharelink.Fetch(context.Background(), nil, created.ShareLink.URL)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := fetched.Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Title != "GitHub" || len(payload.Fields) != 2 || payload.Fields[0].Value != "github-password" || payload.Fields[1].Value != "a1b2" {
		t.Fatalf("payload %+v", payload)
	}
	if encoded, _ := json.Marshal(api.ciphertexts); strings.Contains(string(encoded), "github-password") {
		t.Fatal("the server received the password")
	}

	for _, testCase := range []struct {
		query, failure string
	}{
		{"git", `"git" matches 2 items`},
		{"nothing", `no item matches "nothing"`},
	} {
		request := create
		request.Query = testCase.query
		if failure := test.send(t, request, nil); !strings.Contains(failure, testCase.failure) {
			t.Errorf("%s: %q", testCase.query, failure)
		}
	}
	request := create
	request.ShareLink = &cliprotocol.ShareLinkChange{Fields: []string{"private key"}, ExpiresInSeconds: 86400, MaxViews: 1}
	if failure := test.send(t, request, nil); !strings.Contains(failure, "never leave Hisar") {
		t.Errorf("private key: %q", failure)
	}

	var text cliprotocol.ShareLinkResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeCreateShareLink, ShareLink: &cliprotocol.ShareLinkChange{Text: "deploy token: abc", Title: "Deploy", ExpiresInSeconds: 3600, Passphrase: "correct horse"}}, &text)
	if text.ShareLink.Title != "Deploy" || !text.ShareLink.IsText || !text.ShareLink.HasPassphrase {
		t.Errorf("text link %+v", text.ShareLink)
	}

	var list cliprotocol.ShareLinkListResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListShareLink}, &list)
	if len(list.ShareLinks) != 2 {
		t.Fatalf("list %+v", list)
	}
	for _, link := range list.ShareLinks {
		if link.URL != "" {
			t.Errorf("the list carries a link with its key: %+v", link)
		}
	}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeRevokeShareLink, ShareLinkID: created.ShareLink.ID}, nil)
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeRevokeShareLink, ShareLinkID: created.ShareLink.ID}, nil); !strings.Contains(failure, "already revoked") {
		t.Errorf("second revoke: %q", failure)
	}
	if _, err := sharelink.Fetch(context.Background(), nil, created.ShareLink.URL); err != sharelink.ErrGone {
		t.Errorf("revoked link: %v", err)
	}
	if count := test.approver.getCount(); count != 5 {
		t.Errorf("%d approvals; every share link asks, list and revoke share the session's", count)
	}
}

func TestCommandLineShareLinkDescription(t *testing.T) {
	testService := createUnlockedTestService(t)
	bridge := NewCommandLineBridge(testService.service, clisocket.NewSessionApprovals(0, nil, nil))
	if _, err := testService.service.SaveItem(testService.personalID, ItemDetail{Title: "GitHub", Username: "alice", Password: "pw", URLs: []string{"https://github.com"}}); err != nil {
		t.Fatal(err)
	}
	request := &cliprotocol.Request{Vault: "Personal", Query: "git", ShareLink: &cliprotocol.ShareLinkChange{}}
	if title, labels := bridge.DescribeShareLink(request); title != "GitHub" || strings.Join(labels, ",") != "username,password" {
		t.Errorf("default description %q %v", title, labels)
	}
	request.ShareLink.Fields = []string{"URL"}
	if title, labels := bridge.DescribeShareLink(request); title != "GitHub" || strings.Join(labels, ",") != "website" {
		t.Errorf("description %q %v", title, labels)
	}
	testService.service.Lock()
	if title, labels := bridge.DescribeShareLink(request); title != "git" || strings.Join(labels, ",") != "URL" {
		t.Errorf("locked description %q %v", title, labels)
	}
}

// A vault or item that did not open may have the name or title asked for, so hisar share-link refuses names beside one.
func TestCommandLineItemLookupRefusesNamesAmongUnreadableRecords(t *testing.T) {
	f := createUnlockedTestService(t)
	saveTestItem(t, f.service, f.personalID, ItemDetail{Title: "Deploy", Password: "test-only"})
	if _, _, err := f.service.resolveCommandLineItem("Personal", "Deploy"); err != nil {
		t.Fatalf("lookup among readable records: %v", err)
	}
	judgeCorruptFamilyName(t, f)
	if _, _, err := f.service.resolveCommandLineItem("Personal", "Deploy"); !errors.Is(err, session.ErrIncompleteRead) {
		t.Errorf("vault name lookup beside an unreadable vault name: %v, want an incomplete read", err)
	}

	f = createUnlockedTestService(t)
	saveTestItem(t, f.service, f.personalID, ItemDetail{Title: "Deploy", Password: "test-only"})
	injectJudgeBadItem(t, f)
	if _, _, err := f.service.resolveCommandLineItem("Personal", "Deploy"); !errors.Is(err, session.ErrIncompleteRead) {
		t.Errorf("title lookup beside an unreadable item: %v, want an incomplete read", err)
	}
}

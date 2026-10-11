package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/sharelink"
	"github.com/tryhisar/hisar/desktop/commandline"
)

type fakeShareLinkAPI struct {
	mutex          sync.Mutex
	allLinkByID    map[string]proto.ShareLink
	allCiphertext  map[string]proto.PublicShareLink
	createdCounter int
}

// serveShareLinkAPI answers the share link endpoints and passes everything else to the other test API.
func serveShareLinkAPI(t *testing.T, testService unlockedTestService) *fakeShareLinkAPI {
	t.Helper()
	api := &fakeShareLinkAPI{allLinkByID: map[string]proto.ShareLink{}, allCiphertext: map[string]proto.PublicShareLink{}}
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
	service.unlockedSession.Client.BaseURL = httpServer.URL
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
		var create proto.CreateShareLinkRequest
		json.NewDecoder(request.Body).Decode(&create)
		api.createdCounter++
		id = strings.Repeat(string(rune('A'+api.createdCounter)), 22)
		link := proto.ShareLink{ID: id, EncryptedLabel: create.EncryptedLabel, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Duration(create.ExpiresInSeconds) * time.Second), MaxViews: create.MaxViews, HasPassphrase: create.Passphrase != nil}
		api.allLinkByID[id] = link
		api.allCiphertext[id] = proto.PublicShareLink{Ciphertext: create.Ciphertext, Passphrase: create.Passphrase, ExpiresAt: link.ExpiresAt, MaxViews: link.MaxViews}
		writer.WriteHeader(http.StatusCreated)
		json.NewEncoder(writer).Encode(link)
	case request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/public/"):
		public, isFound := api.allCiphertext[id]
		if !isFound {
			writer.WriteHeader(http.StatusNotFound)
			writer.Write([]byte(`{"error":"not found"}`))
			return
		}
		json.NewEncoder(writer).Encode(public)
	case request.Method == http.MethodGet:
		allLink := []proto.ShareLink{}
		for _, link := range api.allLinkByID {
			allLink = append(allLink, link)
		}
		json.NewEncoder(writer).Encode(allLink)
	case request.Method == http.MethodDelete:
		if _, isFound := api.allLinkByID[id]; !isFound {
			writer.WriteHeader(http.StatusNotFound)
			writer.Write([]byte(`{"error":"not found"}`))
			return
		}
		delete(api.allLinkByID, id)
		delete(api.allCiphertext, id)
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
	if encoded, _ := json.Marshal(api.allCiphertext); strings.Contains(string(encoded), "github-password") {
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
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeCreateShareLink, ShareLink: &cliprotocol.ShareLinkChange{Text: "deploy token: abc", Title: "Deploy", ExpiresInSeconds: 3600, MaxViews: 5, Passphrase: "correct horse"}}, &text)
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
	bridge := CreateCommandLineBridge(testService.service, commandline.CreateSessionApprovals(0, nil))
	if _, err := testService.service.SaveItem(testService.personalID, ItemDetail{Title: "GitHub", Username: "alice", Password: "pw", URLs: []string{"https://github.com"}}); err != nil {
		t.Fatal(err)
	}
	request := &cliprotocol.Request{Vault: "Personal", Query: "git", ShareLink: &cliprotocol.ShareLinkChange{}}
	if title, allLabel := bridge.DescribeShareLink(request); title != "GitHub" || strings.Join(allLabel, ",") != "username,password" {
		t.Errorf("default description %q %v", title, allLabel)
	}
	request.ShareLink.Fields = []string{"URL"}
	if title, allLabel := bridge.DescribeShareLink(request); title != "GitHub" || strings.Join(allLabel, ",") != "website" {
		t.Errorf("description %q %v", title, allLabel)
	}
	testService.service.Lock()
	if title, allLabel := bridge.DescribeShareLink(request); title != "git" || strings.Join(allLabel, ",") != "URL" {
		t.Errorf("locked description %q %v", title, allLabel)
	}
}

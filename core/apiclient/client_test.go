package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
)

func startFakeServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	testServer := httptest.NewServer(handler)
	t.Cleanup(testServer.Close)
	fakeClient := New(testServer.URL + "/")
	fakeClient.Token = "tok"
	return fakeClient
}

func TestRequestShape(t *testing.T) {
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != "PUT" || request.URL.Path != "/api/v1/vaults/v%2F1/items/i1" && request.URL.RawPath != "/api/v1/vaults/v%2F1/items/i1" {
			t.Errorf("unexpected %s %s", request.Method, request.URL)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		if got := request.Header.Get("If-Match"); got != `"rev-0"` {
			t.Errorf("If-Match = %q", got)
		}
		var putRequest apitypes.PutItemRequest
		if err := json.NewDecoder(request.Body).Decode(&putRequest); err != nil || putRequest.Revision != "rev-1" {
			t.Errorf("body: %+v %v", putRequest, err)
		}
		json.NewEncoder(writer).Encode(apitypes.PutItemResponse{Seq: 7})
	})
	seq, current, err := fakeClient.PutItem(context.Background(), "v/1", "i1", "rev-0", &apitypes.PutItemRequest{Revision: "rev-1"})
	if err != nil || seq != 7 || current != nil {
		t.Fatalf("seq=%d cur=%v err=%v", seq, current, err)
	}
}

func TestNewItemHasNoIfMatch(t *testing.T) {
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := request.Header["If-Match"]; ok {
			t.Error("If-Match sent for new item")
		}
		json.NewEncoder(writer).Encode(apitypes.PutItemResponse{Seq: 1})
	})
	if _, _, err := fakeClient.PutItem(context.Background(), "v", "i", "", &apitypes.PutItemRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestConflictReturnsCurrent(t *testing.T) {
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusConflict)
		json.NewEncoder(writer).Encode(apitypes.ConflictResponse{Current: &apitypes.Item{ID: "i", Revision: "rev-9"}})
	})
	_, current, err := fakeClient.PutItem(context.Background(), "v", "i", "rev-0", &apitypes.PutItemRequest{})
	if !IsStatus(err, http.StatusConflict) || current == nil || current.Revision != "rev-9" {
		t.Fatalf("cur=%+v err=%v", current, err)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusForbidden, `{"error":"admin only"}`, "admin only"},
		{http.StatusInternalServerError, `not json`, "Internal Server Error"},
		{http.StatusConflict, `{"error":"vault exists"}`, "vault exists"},
	}
	for _, testCase := range cases {
		fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(testCase.status)
			writer.Write([]byte(testCase.body))
		})
		_, err := fakeClient.GetAccount(context.Background())
		apiError, ok := err.(*APIError)
		if !ok || apiError.Status != testCase.status || apiError.Message != testCase.want {
			t.Errorf("%d: got %v", testCase.status, err)
		}
	}
}

func TestQueryEscaping(t *testing.T) {
	preloginClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if got := request.URL.Query().Get("email"); got != "a+b@example.com" {
			t.Errorf("email = %q", got)
		}
		if request.Header.Get("Authorization") != "Bearer tok" {
			t.Error("missing token")
		}
		json.NewEncoder(writer).Encode(apitypes.PreloginResponse{Salt: []byte{1}})
	})
	prelogin, err := preloginClient.FetchPrelogin(context.Background(), "a+b@example.com")
	if err != nil || len(prelogin.Salt) != 1 {
		t.Fatal(prelogin, err)
	}

	changeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("since") != "42" || request.URL.Query().Get("limit") != "10" {
			t.Errorf("query = %s", request.URL.RawQuery)
		}
		json.NewEncoder(writer).Encode(apitypes.ChangesResponse{NextSeq: 50})
	})
	changeResponse, err := changeClient.ListChanges(context.Background(), "v", 42, 10)
	if err != nil || changeResponse.NextSeq != 50 {
		t.Fatal(changeResponse, err)
	}
}

func TestIsUnreachable(t *testing.T) {
	stoppedServer := httptest.NewServer(http.NotFoundHandler())
	stoppedServer.Close()
	if _, err := New(stoppedServer.URL).GetAccount(context.Background()); !IsUnreachable(err) {
		t.Errorf("connection refused: %v", err)
	}
	cases := map[int]bool{http.StatusBadGateway: true, http.StatusServiceUnavailable: true, http.StatusGatewayTimeout: true, http.StatusUnauthorized: false, http.StatusInternalServerError: false}
	for status, want := range cases {
		fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) { writer.WriteHeader(status) })
		if _, err := fakeClient.GetAccount(context.Background()); IsUnreachable(err) != want {
			t.Errorf("%d: unreachable = %v", status, !want)
		}
	}
}

func TestIsStatusMatchesWrappedErrors(t *testing.T) {
	apiError := &APIError{Status: http.StatusForbidden}
	if !IsStatus(fmt.Errorf("vault v1: %w", apiError), http.StatusForbidden) {
		t.Error("wrapped API error not matched")
	}
	if IsStatus(fmt.Errorf("vault v1: %w", apiError), http.StatusNotFound) || IsStatus(errors.New("plain"), http.StatusForbidden) {
		t.Error("matched the wrong status or a plain error")
	}
}

// startRedirectTarget is another origin on the same host; it records the Authorization header it receives.
func startRedirectTarget(t *testing.T) (string, *[]string) {
	t.Helper()
	var authorizations []string
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorizations = append(authorizations, request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "application/json")
		writer.Write([]byte(`{}`))
	}))
	t.Cleanup(target.Close)
	return target.URL, &authorizations
}

func TestRedirectsNeverDowngradeTLSOrCarryTheBearerToAnotherOrigin(t *testing.T) {
	targetURL, authorizations := startRedirectTarget(t)
	redirectTo := func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, targetURL+request.URL.Path, http.StatusFound)
	}
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(redirectTo))
	t.Cleanup(tlsServer.Close)
	tlsClient := New(tlsServer.URL)
	tlsClient.HTTP.Transport = tlsServer.Client().Transport
	tlsClient.Token = "tok"
	ctx := context.Background()

	_, err := tlsClient.GetAccount(ctx)
	if !errors.Is(err, ErrInsecureRedirect) || IsUnreachable(err) {
		t.Errorf("https redirected to http: %v", err)
	}
	streamErr := tlsClient.StreamEvents(ctx, nil, func(Event) {})
	if !errors.Is(streamErr, ErrInsecureRedirect) {
		t.Errorf("event stream redirected to http: %v", streamErr)
	}
	if len(*authorizations) != 0 {
		t.Errorf("the http origin received %q", *authorizations)
	}

	otherPortClient := startFakeServer(t, redirectTo)
	if _, err := otherPortClient.GetAccount(ctx); err != nil {
		t.Fatal(err)
	}
	if len(*authorizations) != 1 || (*authorizations)[0] != "" {
		t.Errorf("another port of the same host received %q", *authorizations)
	}
}

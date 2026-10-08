package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tryhisar/hisar/core/proto"
)

func fakeServer(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	c := New(ts.URL + "/")
	c.Token = "tok"
	return c
}

func TestRequestShape(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.URL.Path != "/api/v1/vaults/v%2F1/items/i1" && r.URL.RawPath != "/api/v1/vaults/v%2F1/items/i1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("If-Match"); got != `"rev-0"` {
			t.Errorf("If-Match = %q", got)
		}
		var req proto.PutItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Revision != "rev-1" {
			t.Errorf("body: %+v %v", req, err)
		}
		json.NewEncoder(w).Encode(proto.PutItemResponse{Seq: 7})
	})
	seq, cur, err := c.PutItem(context.Background(), "v/1", "i1", "rev-0", &proto.PutItemRequest{Revision: "rev-1"})
	if err != nil || seq != 7 || cur != nil {
		t.Fatalf("seq=%d cur=%v err=%v", seq, cur, err)
	}
}

func TestNewItemHasNoIfMatch(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["If-Match"]; ok {
			t.Error("If-Match sent for new item")
		}
		json.NewEncoder(w).Encode(proto.PutItemResponse{Seq: 1})
	})
	if _, _, err := c.PutItem(context.Background(), "v", "i", "", &proto.PutItemRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestConflictReturnsCurrent(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(proto.ConflictResponse{Current: &proto.Item{ID: "i", Revision: "rev-9"}})
	})
	_, cur, err := c.PutItem(context.Background(), "v", "i", "rev-0", &proto.PutItemRequest{})
	if !IsStatus(err, http.StatusConflict) || cur == nil || cur.Revision != "rev-9" {
		t.Fatalf("cur=%+v err=%v", cur, err)
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
	for _, tc := range cases {
		c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		})
		_, err := c.Account(context.Background())
		e, ok := err.(*APIError)
		if !ok || e.Status != tc.status || e.Message != tc.want {
			t.Errorf("%d: got %v", tc.status, err)
		}
	}
}

func TestQueryEscaping(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("email"); got != "a+b@example.com" {
			t.Errorf("email = %q", got)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Error("missing token")
		}
		json.NewEncoder(w).Encode(proto.PreloginResponse{Salt: []byte{1}})
	})
	pre, err := c.Prelogin(context.Background(), "a+b@example.com")
	if err != nil || len(pre.Salt) != 1 {
		t.Fatal(pre, err)
	}

	c2 := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") != "42" || r.URL.Query().Get("limit") != "10" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(proto.ChangesResponse{NextSeq: 50})
	})
	ch, err := c2.Changes(context.Background(), "v", 42, 10)
	if err != nil || ch.NextSeq != 50 {
		t.Fatal(ch, err)
	}
}

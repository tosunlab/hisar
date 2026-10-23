package apiclient

import (
	"context"
	"encoding/json"
	"github.com/tryhisar/hisar/core/apitypes"
	"net/http"
	"strings"
	"testing"
)

func TestCollectionTraversalRestartsWithoutPublishingPartialResults(t *testing.T) {
	starts, requests := 0, 0
	client := startFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get(apitypes.CollectionPageHeader) != apitypes.CollectionPageVersion {
			t.Error("feature not announced")
		}
		if r.URL.Query().Get("cursor") == "" {
			starts++
			json.NewEncoder(w).Encode(apitypes.CollectionPage{Version: "1", Records: []apitypes.CollectionRecord{{Key: "a", Value: json.RawMessage(`{"id":"a"}`)}}, NextCursor: "next"})
			return
		}
		if starts == 1 {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(apitypes.Error{Error: apitypes.CollectionChangedMessage})
			return
		}
		json.NewEncoder(w).Encode(apitypes.CollectionPage{Version: "1", Records: []apitypes.CollectionRecord{{Key: "b", Value: json.RawMessage(`{"id":"b"}`)}}})
	})
	vaults, err := client.ListVaults(context.Background())
	if err != nil || len(vaults) != 2 || vaults[0].ID != "a" || vaults[1].ID != "b" || starts != 2 || requests != 4 {
		t.Fatalf("%+v %v starts=%d requests=%d", vaults, err, starts, requests)
	}
}
func TestCollectionRejectsIncompleteClaimedPages(t *testing.T) {
	for _, body := range []string{`{"version":"2","records":[]}`, `{"records":[]}`, `{"version":"1"}`, `{"version":"1","records":[],"nextCursor":"same"}`, `{"version":"1","records":[{"key":"a","value":{}},{"key":"a","value":{}}]}`} {
		t.Run(body, func(t *testing.T) {
			client := startFakeServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
			vaults, err := client.ListVaults(context.Background())
			if err == nil || len(vaults) != 0 {
				t.Fatalf("partial claimed success %+v %v", vaults, err)
			}
		})
	}
}
func TestCompleteLegacyCollectionRemainsReadable(t *testing.T) {
	client := startFakeServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`[{"id":"old-a"},{"id":"old-b"}]`)) })
	vaults, err := client.ListVaults(context.Background())
	if err != nil || len(vaults) != 2 {
		t.Fatalf("legacy %+v %v", vaults, err)
	}
}
func TestCollectionChurnHasBoundedRestartAndNoPartialResult(t *testing.T) {
	starts := 0
	client := startFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			starts++
			w.Write([]byte(`{"version":"1","records":[{"key":"a","value":{}}],"nextCursor":"next"}`))
			return
		}
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(apitypes.Error{Error: apitypes.CollectionChangedMessage})
	})
	vaults, err := client.ListVaults(context.Background())
	if starts != 3 || err == nil || !strings.Contains(err.Error(), "changed") || len(vaults) != 0 {
		t.Fatalf("starts=%d result=%+v %v", starts, vaults, err)
	}
}

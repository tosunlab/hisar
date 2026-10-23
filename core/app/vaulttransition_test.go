package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"sort"

	"github.com/tryhisar/hisar/core/apitypes"
)

type fakeVaultTransition struct {
	apitypes.VaultTransition
	vaultID string
	items   map[string]apitypes.RotatedItem
	access  map[string]apitypes.RotatedAccess
}

func (server *fakeAPIServer) addVaultTransitionRoutes(mux *http.ServeMux) {
	operations := map[string]*fakeVaultTransition{}
	fail := func(w http.ResponseWriter, status int, message string) {
		writeJSON(w, status, apitypes.Error{Error: message})
	}
	mux.HandleFunc("GET /api/v1/vaults/{vaultID}/transitions", func(w http.ResponseWriter, r *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if server.getVaultRole(r.PathValue("vaultID")) != apitypes.VaultOwner {
			fail(w, 403, "owner only")
			return
		}
		writeJSON(w, 200, struct {
			Protocol int `json:"protocol"`
		}{apitypes.VaultTransitionProtocol})
	})
	mux.HandleFunc("POST /api/v1/vaults/{vaultID}/transitions", func(w http.ResponseWriter, r *http.Request) {
		var request apitypes.BeginVaultTransitionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			fail(w, 400, err.Error())
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID := r.PathValue("vaultID")
		if server.getVaultRole(vaultID) != apitypes.VaultOwner {
			fail(w, 403, "owner only")
			return
		}
		if request.KeyGeneration != server.getKeyGeneration(vaultID) {
			fail(w, 409, "the vault key changed")
			return
		}
		if request.Kind == "delete-move" && server.refusesKeyGeneration(w, request.TargetID, request.TargetKeyGeneration) {
			return
		}
		op := &fakeVaultTransition{VaultTransition: apitypes.VaultTransition{Protocol: 1, ID: request.ID, Kind: request.Kind, State: "staging",
			Source: apitypes.VaultSnapshot{KeyGeneration: request.KeyGeneration}, TargetID: request.TargetID}, vaultID: vaultID,
			items: map[string]apitypes.RotatedItem{}, access: map[string]apitypes.RotatedAccess{}}
		for _, vault := range server.vaults {
			if vault.ID == vaultID {
				op.SourceName = slices.Clone(vault.EncName)
			}
		}
		if request.TargetID != "" {
			op.Target = &apitypes.VaultSnapshot{KeyGeneration: request.TargetKeyGeneration}
		}
		operations[op.ID] = op
		writeJSON(w, 201, op.VaultTransition)
	})
	mux.HandleFunc("GET /api/v1/vaults/{vaultID}/transitions/{id}", func(w http.ResponseWriter, r *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		op := operations[r.PathValue("id")]
		if op == nil {
			fail(w, 404, "not found")
			return
		}
		writeJSON(w, 200, op.VaultTransition)
	})
	mux.HandleFunc("GET /api/v1/vaults/{vaultID}/transitions/{id}/items", func(w http.ResponseWriter, r *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		op := operations[r.PathValue("id")]
		if op == nil {
			fail(w, 404, "not found")
			return
		}
		ids := []string{}
		for id, item := range server.itemsByVaultID[op.vaultID] {
			if id > r.URL.Query().Get("after") && (op.Kind == "rotate" || !item.Deleted) {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		page := apitypes.VaultTransitionPage{Items: []apitypes.Item{}}
		used := 0
		for _, id := range ids {
			if len(page.Items) >= 100 || used >= 4<<20 {
				page.NextCursor = page.Items[len(page.Items)-1].ID
				break
			}
			item := server.itemsByVaultID[op.vaultID][id]
			page.Items = append(page.Items, item)
			used += len(item.Overview) + len(item.Details)
		}
		writeJSON(w, 200, page)
	})
	mux.HandleFunc("POST /api/v1/vaults/{vaultID}/transitions/{id}/items", func(w http.ResponseWriter, r *http.Request) {
		var request apitypes.StageVaultItemsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			fail(w, 400, err.Error())
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		op := operations[r.PathValue("id")]
		if op == nil {
			fail(w, 404, "not found")
			return
		}
		for _, item := range request.Items {
			op.items[item.ID] = item
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/v1/vaults/{vaultID}/transitions/{id}/access", func(w http.ResponseWriter, r *http.Request) {
		var request apitypes.StageVaultAccessRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			fail(w, 400, err.Error())
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		op := operations[r.PathValue("id")]
		if op == nil {
			fail(w, 404, "not found")
			return
		}
		for _, access := range request.Accesses {
			op.access[access.AccountID] = access
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/v1/vaults/{vaultID}/transitions/{id}/abort", func(w http.ResponseWriter, r *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if op := operations[r.PathValue("id")]; op != nil && op.State != "committed" {
			op.State = "aborted"
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/v1/vaults/{vaultID}/transitions/{id}/commit", func(w http.ResponseWriter, r *http.Request) {
		var request apitypes.CommitVaultTransitionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			fail(w, 400, err.Error())
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		op := operations[r.PathValue("id")]
		if op == nil {
			fail(w, 404, "not found")
			return
		}
		if op.State == "committed" {
			writeJSON(w, 200, op.VaultTransition)
			return
		}
		if server.getVaultRole(op.vaultID) != apitypes.VaultOwner {
			fail(w, 403, "owner only")
			return
		}
		if server.getKeyGeneration(op.vaultID) != op.Source.KeyGeneration {
			fail(w, 409, "key changed")
			return
		}
		items := server.itemsByVaultID[op.vaultID]
		count := 0
		for id, stored := range items {
			if op.Kind == "delete-move" && stored.Deleted {
				continue
			}
			count++
			staged, ok := op.items[id]
			if !ok || staged.BaseRevision != stored.Revision {
				fail(w, 409, "items changed")
				return
			}
		}
		if count != len(op.items) {
			fail(w, 409, "items changed")
			return
		}
		if op.Kind == "rotate" {
			accesses := server.accessesByVaultID[op.vaultID]
			if len(op.access) != len(accesses) {
				fail(w, 400, "rotation must retain current members")
				return
			}
			for _, access := range accesses {
				if staged, ok := op.access[access.AccountID]; !ok || staged.Role != access.Role {
					fail(w, 400, "rotation must retain current roles")
					return
				}
			}
			for index := range server.vaults {
				vault := &server.vaults[index]
				if vault.ID != op.vaultID {
					continue
				}
				if !bytes.Equal(vault.EncName, op.SourceName) {
					fail(w, 409, "name changed")
					return
				}
				vault.EncName = request.EncName
				vault.KeyGeneration++
				vault.NeedsRotation = false
				vault.WrappedKey = op.access[server.accountID].WrappedKey
			}
			server.accessesByVaultID[op.vaultID] = nil
			for _, access := range op.access {
				server.accessesByVaultID[op.vaultID] = append(server.accessesByVaultID[op.vaultID], access)
			}
		}
		target := op.vaultID
		if op.Kind == "delete-move" {
			target = op.TargetID
			if server.refusesKeyGeneration(w, target, op.Target.KeyGeneration) {
				return
			}
		}
		for _, staged := range op.items {
			server.seq++
			server.itemsByVaultID[target][staged.ID] = apitypes.Item{ID: staged.ID, Revision: staged.Revision, Seq: server.seq, Deleted: staged.Deleted, Overview: staged.Overview, Details: staged.Details}
		}
		if op.Kind == "delete-move" {
			server.removeVault(op.vaultID)
		}
		op.State = "committed"
		op.ItemCount = int64(len(op.items))
		writeJSON(w, 200, op.VaultTransition)
	})
}

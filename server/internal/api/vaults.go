package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	maxEncName    = 1024
	maxWrappedKey = 4096
	maxOverview   = 64 << 10
	maxDetails    = 1 << 20
	defaultLimit  = 500
	maxLimit      = 1000
)

func validUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil && len(s) == 36
}

func validVaultRole(r string) bool {
	return r == proto.VaultOwner || r == proto.VaultEditor || r == proto.VaultReader
}

// vaultRole returns the caller's role, writing 404 for missing vaults and non-members alike.
func (s *Server) vaultRole(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("vault")
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not found")
		return "", false
	}
	role, err := s.store.VaultRole(r.Context(), id, sessionOf(r).AccountID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return "", false
	}
	if err != nil {
		internalError(w, err)
		return "", false
	}
	return role, true
}

func (s *Server) listVaults(w http.ResponseWriter, r *http.Request) {
	vs, err := s.store.ListVaults(r.Context(), sessionOf(r).AccountID)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]proto.Vault, 0, len(vs))
	for _, v := range vs {
		out = append(out, proto.Vault{ID: v.ID, Type: v.Type, EncName: v.EncName, Seq: v.Seq, Role: v.Role, WrappedKey: v.WrappedKey})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createVault(w http.ResponseWriter, r *http.Request) {
	sess := sessionOf(r)
	var req proto.CreateVaultRequest
	if !decode(w, r, &req) {
		return
	}
	if !validUUID(req.ID) || (req.Type != proto.VaultPersonal && req.Type != proto.VaultShared) ||
		len(req.EncName) == 0 || len(req.EncName) > maxEncName ||
		len(req.WrappedKey) == 0 || len(req.WrappedKey) > maxWrappedKey {
		writeError(w, http.StatusBadRequest, "invalid vault")
		return
	}
	v := &store.Vault{ID: req.ID, Type: req.Type, EncName: req.EncName, CreatedBy: sess.AccountID}
	switch err := s.store.CreateVault(r.Context(), v, req.WrappedKey); {
	case errors.Is(err, store.ErrExists):
		writeError(w, http.StatusConflict, "vault exists")
	case err != nil:
		internalError(w, err)
	default:
		s.audit(r, sess.AccountID, sess.DeviceID, "vault_created", map[string]string{"vault": v.ID, "type": v.Type})
		writeJSON(w, http.StatusCreated, proto.Vault{ID: v.ID, Type: v.Type, EncName: v.EncName, Role: proto.VaultOwner, WrappedKey: req.WrappedKey})
	}
}

func (s *Server) listVaultMembers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.vaultRole(w, r); !ok {
		return
	}
	ms, err := s.store.ListMembers(r.Context(), r.PathValue("vault"))
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]proto.VaultMember, 0, len(ms))
	for _, m := range ms {
		out = append(out, proto.VaultMember{AccountID: m.AccountID, Role: m.Role, GrantedBy: m.GrantedBy})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setAccess(w http.ResponseWriter, r *http.Request) {
	sess := sessionOf(r)
	role, ok := s.vaultRole(w, r)
	if !ok {
		return
	}
	if role != proto.VaultOwner {
		writeError(w, http.StatusForbidden, "owner only")
		return
	}
	target := r.PathValue("account")
	if target == sess.AccountID {
		writeError(w, http.StatusBadRequest, "cannot change own access")
		return
	}
	var req proto.SetAccessRequest
	if !decode(w, r, &req) {
		return
	}
	if !validVaultRole(req.Role) || len(req.WrappedKey) == 0 || len(req.WrappedKey) > maxWrappedKey {
		writeError(w, http.StatusBadRequest, "invalid access")
		return
	}
	if !crypt.ValidAccountID(target) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if _, err := s.store.AccountByID(r.Context(), target); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	vaultID := r.PathValue("vault")
	if err := s.store.SetAccess(r.Context(), vaultID, store.Member{AccountID: target, Role: req.Role, WrappedKey: req.WrappedKey, GrantedBy: sess.AccountID}); err != nil {
		internalError(w, err)
		return
	}
	s.audit(r, sess.AccountID, sess.DeviceID, "vault_access_set", map[string]string{"vault": vaultID, "account": target, "role": req.Role})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeAccess(w http.ResponseWriter, r *http.Request) {
	sess := sessionOf(r)
	role, ok := s.vaultRole(w, r)
	if !ok {
		return
	}
	target := r.PathValue("account")
	switch {
	case target == sess.AccountID && role == proto.VaultOwner:
		writeError(w, http.StatusBadRequest, "owner cannot leave vault")
		return
	case target != sess.AccountID && role != proto.VaultOwner:
		writeError(w, http.StatusForbidden, "owner only")
		return
	}
	vaultID := r.PathValue("vault")
	switch err := s.store.RemoveAccess(r.Context(), vaultID, target); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case err != nil:
		internalError(w, err)
	default:
		s.audit(r, sess.AccountID, sess.DeviceID, "vault_access_removed", map[string]string{"vault": vaultID, "account": target})
		w.WriteHeader(http.StatusNoContent)
	}
}

func toProtoItem(it *store.Item) proto.Item {
	return proto.Item{ID: it.ID, Revision: it.Revision, Seq: it.Seq, Deleted: it.Deleted, Overview: it.Overview, Details: it.Details, UpdatedAt: it.UpdatedAt, UpdatedBy: it.UpdatedBy}
}

func (s *Server) changes(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.vaultRole(w, r); !ok {
		return
	}
	q := r.URL.Query()
	since, err := strconv.ParseInt(q.Get("since"), 10, 64)
	if q.Get("since") == "" {
		since, err = 0, nil
	}
	if err != nil || since < 0 {
		writeError(w, http.StatusBadRequest, "invalid since")
		return
	}
	limit := defaultLimit
	if l := q.Get("limit"); l != "" {
		if limit, err = strconv.Atoi(l); err != nil || limit < 1 || limit > maxLimit {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
	}

	items, vaultSeq, err := s.store.Changes(r.Context(), r.PathValue("vault"), since, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	// NextSeq below the client's since signals a server rollback; it is not clamped.
	resp := proto.ChangesResponse{Items: make([]proto.Item, 0, len(items)), NextSeq: vaultSeq}
	for i := range items {
		resp.Items = append(resp.Items, toProtoItem(&items[i]))
	}
	if len(items) == limit {
		resp.More = true
		resp.NextSeq = items[len(items)-1].Seq
	} else if len(items) > 0 {
		resp.NextSeq = max(vaultSeq, items[len(items)-1].Seq)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) putItem(w http.ResponseWriter, r *http.Request) {
	sess := sessionOf(r)
	role, ok := s.vaultRole(w, r)
	if !ok {
		return
	}
	if role == proto.VaultReader {
		writeError(w, http.StatusForbidden, "read only")
		return
	}
	itemID := r.PathValue("item")
	var req proto.PutItemRequest
	if !decode(w, r, &req) {
		return
	}
	base := strings.Trim(strings.TrimPrefix(r.Header.Get("If-Match"), "W/"), `"`)
	if !validUUID(itemID) || !validUUID(req.Revision) || (base != "" && !validUUID(base)) ||
		len(req.Overview) == 0 || len(req.Overview) > maxOverview ||
		len(req.Details) == 0 || len(req.Details) > maxDetails {
		writeError(w, http.StatusBadRequest, "invalid item")
		return
	}

	it := &store.Item{ID: itemID, Revision: req.Revision, Deleted: req.Deleted, Overview: req.Overview, Details: req.Details, UpdatedBy: sess.AccountID}
	current, err := s.store.PutItem(r.Context(), r.PathValue("vault"), base, it)
	switch {
	case errors.Is(err, store.ErrConflict):
		resp := proto.ConflictResponse{}
		if current != nil {
			c := toProtoItem(current)
			resp.Current = &c
		}
		writeJSON(w, http.StatusConflict, resp)
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, proto.PutItemResponse{Seq: it.Seq})
	}
}

func (s *Server) itemHistory(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.vaultRole(w, r); !ok {
		return
	}
	itemID := r.PathValue("item")
	if !validUUID(itemID) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	items, err := s.store.ItemHistory(r.Context(), r.PathValue("vault"), itemID)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]proto.Item, 0, len(items))
	for i := range items {
		out = append(out, toProtoItem(&items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

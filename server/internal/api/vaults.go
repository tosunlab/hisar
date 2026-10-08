package api

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
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

// vaultRole returns the caller's role; missing vaults and non-members both get 404.
func (s *Server) vaultRole(c fiber.Ctx) (string, error) {
	id := c.Params("vault")
	if !validUUID(id) {
		return "", errNotFound
	}
	role, err := s.store.VaultRole(c.Context(), id, sessionOf(c).AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return "", errNotFound
	}
	return role, err
}

func (s *Server) listVaults(c fiber.Ctx) error {
	vs, err := s.store.ListVaults(c.Context(), sessionOf(c).AccountID)
	if err != nil {
		return err
	}
	out := make([]proto.Vault, 0, len(vs))
	for _, v := range vs {
		out = append(out, proto.Vault{ID: v.ID, Type: v.Type, EncName: v.EncName, Seq: v.Seq, Role: v.Role, WrappedKey: v.WrappedKey})
	}
	return c.JSON(out)
}

func (s *Server) createVault(c fiber.Ctx) error {
	sess := sessionOf(c)
	var req proto.CreateVaultRequest
	if err := decode(c, &req); err != nil {
		return err
	}
	if !validUUID(req.ID) || (req.Type != proto.VaultPersonal && req.Type != proto.VaultShared) ||
		len(req.EncName) == 0 || len(req.EncName) > maxEncName ||
		len(req.WrappedKey) == 0 || len(req.WrappedKey) > maxWrappedKey {
		return apiError(fiber.StatusBadRequest, "invalid vault")
	}
	v := &store.Vault{ID: req.ID, Type: req.Type, EncName: req.EncName, CreatedBy: sess.AccountID}
	switch err := s.store.CreateVault(c.Context(), v, req.WrappedKey); {
	case errors.Is(err, store.ErrExists):
		return apiError(fiber.StatusConflict, "vault exists")
	case err != nil:
		return err
	}
	s.audit(c, sess.AccountID, sess.DeviceID, "vault_created", map[string]string{"vault": v.ID, "type": v.Type})
	return c.Status(fiber.StatusCreated).JSON(proto.Vault{ID: v.ID, Type: v.Type, EncName: v.EncName, Role: proto.VaultOwner, WrappedKey: req.WrappedKey})
}

func (s *Server) listVaultMembers(c fiber.Ctx) error {
	if _, err := s.vaultRole(c); err != nil {
		return err
	}
	ms, err := s.store.ListMembers(c.Context(), c.Params("vault"))
	if err != nil {
		return err
	}
	out := make([]proto.VaultMember, 0, len(ms))
	for _, m := range ms {
		out = append(out, proto.VaultMember{AccountID: m.AccountID, Role: m.Role, GrantedBy: m.GrantedBy})
	}
	return c.JSON(out)
}

func (s *Server) setAccess(c fiber.Ctx) error {
	sess := sessionOf(c)
	role, err := s.vaultRole(c)
	if err != nil {
		return err
	}
	if role != proto.VaultOwner {
		return apiError(fiber.StatusForbidden, "owner only")
	}
	target := c.Params("account")
	if target == sess.AccountID {
		return apiError(fiber.StatusBadRequest, "cannot change own access")
	}
	var req proto.SetAccessRequest
	if err := decode(c, &req); err != nil {
		return err
	}
	if !validVaultRole(req.Role) || len(req.WrappedKey) == 0 || len(req.WrappedKey) > maxWrappedKey {
		return apiError(fiber.StatusBadRequest, "invalid access")
	}
	errNoAccount := apiError(fiber.StatusNotFound, "account not found")
	if !crypt.ValidAccountID(target) {
		return errNoAccount
	}
	if _, err := s.store.AccountByID(c.Context(), target); errors.Is(err, store.ErrNotFound) {
		return errNoAccount
	} else if err != nil {
		return err
	}
	vaultID := c.Params("vault")
	if err := s.store.SetAccess(c.Context(), vaultID, store.Member{AccountID: target, Role: req.Role, WrappedKey: req.WrappedKey, GrantedBy: sess.AccountID}); err != nil {
		return err
	}
	s.audit(c, sess.AccountID, sess.DeviceID, "vault_access_set", map[string]string{"vault": vaultID, "account": target, "role": req.Role})
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) removeAccess(c fiber.Ctx) error {
	sess := sessionOf(c)
	role, err := s.vaultRole(c)
	if err != nil {
		return err
	}
	target := c.Params("account")
	switch {
	case target == sess.AccountID && role == proto.VaultOwner:
		return apiError(fiber.StatusBadRequest, "owner cannot leave vault")
	case target != sess.AccountID && role != proto.VaultOwner:
		return apiError(fiber.StatusForbidden, "owner only")
	}
	vaultID := c.Params("vault")
	switch err := s.store.RemoveAccess(c.Context(), vaultID, target); {
	case errors.Is(err, store.ErrNotFound):
		return errNotFound
	case err != nil:
		return err
	}
	s.audit(c, sess.AccountID, sess.DeviceID, "vault_access_removed", map[string]string{"vault": vaultID, "account": target})
	return c.SendStatus(fiber.StatusNoContent)
}

func toProtoItem(it *store.Item) proto.Item {
	return proto.Item{ID: it.ID, Revision: it.Revision, Seq: it.Seq, Deleted: it.Deleted, Overview: it.Overview, Details: it.Details, UpdatedAt: it.UpdatedAt, UpdatedBy: it.UpdatedBy}
}

func (s *Server) changes(c fiber.Ctx) error {
	if _, err := s.vaultRole(c); err != nil {
		return err
	}
	var since int64
	if q := c.Query("since"); q != "" {
		var err error
		if since, err = strconv.ParseInt(q, 10, 64); err != nil || since < 0 {
			return apiError(fiber.StatusBadRequest, "invalid since")
		}
	}
	limit := defaultLimit
	if q := c.Query("limit"); q != "" {
		var err error
		if limit, err = strconv.Atoi(q); err != nil || limit < 1 || limit > maxLimit {
			return apiError(fiber.StatusBadRequest, "invalid limit")
		}
	}

	items, vaultSeq, err := s.store.Changes(c.Context(), c.Params("vault"), since, limit)
	if err != nil {
		return err
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
	return c.JSON(resp)
}

func (s *Server) putItem(c fiber.Ctx) error {
	sess := sessionOf(c)
	role, err := s.vaultRole(c)
	if err != nil {
		return err
	}
	if role == proto.VaultReader {
		return apiError(fiber.StatusForbidden, "read only")
	}
	itemID := c.Params("item")
	var req proto.PutItemRequest
	if err := decode(c, &req); err != nil {
		return err
	}
	base := strings.Trim(strings.TrimPrefix(c.Get("If-Match"), "W/"), `"`)
	if !validUUID(itemID) || !validUUID(req.Revision) || (base != "" && !validUUID(base)) ||
		len(req.Overview) == 0 || len(req.Overview) > maxOverview ||
		len(req.Details) == 0 || len(req.Details) > maxDetails {
		return apiError(fiber.StatusBadRequest, "invalid item")
	}

	it := &store.Item{ID: itemID, Revision: req.Revision, Deleted: req.Deleted, Overview: req.Overview, Details: req.Details, UpdatedBy: sess.AccountID}
	current, err := s.store.PutItem(c.Context(), c.Params("vault"), base, it)
	switch {
	case errors.Is(err, store.ErrConflict):
		resp := proto.ConflictResponse{}
		if current != nil {
			cur := toProtoItem(current)
			resp.Current = &cur
		}
		return c.Status(fiber.StatusConflict).JSON(resp)
	case err != nil:
		return err
	}
	return c.JSON(proto.PutItemResponse{Seq: it.Seq})
}

func (s *Server) itemHistory(c fiber.Ctx) error {
	if _, err := s.vaultRole(c); err != nil {
		return err
	}
	itemID := c.Params("item")
	if !validUUID(itemID) {
		return errNotFound
	}
	items, err := s.store.ItemHistory(c.Context(), c.Params("vault"), itemID)
	if err != nil {
		return err
	}
	out := make([]proto.Item, 0, len(items))
	for i := range items {
		out = append(out, toProtoItem(&items[i]))
	}
	return c.JSON(out)
}

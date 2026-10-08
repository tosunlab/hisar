package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/store"
)

func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.AccountByID(r.Context(), sessionOf(r).AccountID)
	if err != nil {
		internalError(w, err)
		return
	}
	var p crypt.Argon2Params
	if err := json.Unmarshal(a.Argon2Params, &p); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, proto.Account{
		AccountID:  a.ID,
		Email:      a.Email,
		Name:       a.Name,
		Role:       a.Role,
		Argon2:     p,
		Salt:       a.Salt,
		EncKeyset:  a.EncKeyset,
		PublicKeys: publicKeys(a),
	})
}

func publicKeys(a *store.Account) crypt.PublicKeys {
	return crypt.PublicKeys{KEM: a.KEMPub, Sign: a.SignPub, KEMSig: a.KEMSig}
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	accts, err := s.store.ListAccounts(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]proto.Member, 0, len(accts))
	for i := range accts {
		a := &accts[i]
		out = append(out, proto.Member{AccountID: a.ID, Email: a.Email, Name: a.Name, Role: a.Role, PublicKeys: publicKeys(a)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	sess := sessionOf(r)
	if sess.Role != proto.RoleAdmin {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}
	var req proto.InviteRequest
	if !decode(w, r, &req) {
		return
	}
	email := normalizeEmail(req.Email)
	if !strings.Contains(email, "@") || (req.Role != proto.RoleAdmin && req.Role != proto.RoleMember) {
		writeError(w, http.StatusBadRequest, "invalid invite")
		return
	}
	resp, err := s.CreateInvite(r.Context(), email, req.Role, &sess.AccountID)
	if err != nil {
		internalError(w, err)
		return
	}
	s.audit(r, sess.AccountID, sess.DeviceID, "invite", map[string]string{"email": email, "role": req.Role})
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) CreateInvite(ctx context.Context, email, role string, createdBy *string) (*proto.InviteResponse, error) {
	token := newToken()
	if err := s.store.CreateInvite(ctx, hashToken(token), email, role, createdBy, s.cfg.InviteTTL); err != nil {
		return nil, err
	}
	return &proto.InviteResponse{Token: token, URL: strings.TrimRight(s.cfg.PublicURL, "/") + "/invite/" + token}, nil
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	sess := sessionOf(r)
	devs, err := s.store.ListDevices(r.Context(), sess.AccountID)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]proto.Device, 0, len(devs))
	for _, d := range devs {
		out = append(out, proto.Device{ID: d.ID, Name: d.Name, Platform: d.Platform, CreatedAt: d.CreatedAt, LastSeenAt: d.LastSeenAt, Current: d.ID == sess.DeviceID})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	sess := sessionOf(r)
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	switch err := s.store.RevokeDevice(r.Context(), sess.AccountID, id); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case err != nil:
		internalError(w, err)
	default:
		s.audit(r, sess.AccountID, sess.DeviceID, "device_revoked", map[string]string{"device": id})
		w.WriteHeader(http.StatusNoContent)
	}
}

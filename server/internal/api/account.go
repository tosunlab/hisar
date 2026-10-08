package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/store"
)

func (s *Server) getAccount(c fiber.Ctx) error {
	a, err := s.store.AccountByID(c.Context(), sessionOf(c).AccountID)
	if err != nil {
		return err
	}
	var p crypt.Argon2Params
	if err := json.Unmarshal(a.Argon2Params, &p); err != nil {
		return err
	}
	return c.JSON(proto.Account{
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

func (s *Server) listMembers(c fiber.Ctx) error {
	accts, err := s.store.ListAccounts(c.Context())
	if err != nil {
		return err
	}
	out := make([]proto.Member, 0, len(accts))
	for i := range accts {
		a := &accts[i]
		out = append(out, proto.Member{AccountID: a.ID, Email: a.Email, Name: a.Name, Role: a.Role, PublicKeys: publicKeys(a)})
	}
	return c.JSON(out)
}

func (s *Server) createInvite(c fiber.Ctx) error {
	sess := sessionOf(c)
	if sess.Role != proto.RoleAdmin {
		return apiError(fiber.StatusForbidden, "admin only")
	}
	var req proto.InviteRequest
	if err := decode(c, &req); err != nil {
		return err
	}
	email := normalizeEmail(req.Email)
	if !strings.Contains(email, "@") || (req.Role != proto.RoleAdmin && req.Role != proto.RoleMember) {
		return apiError(fiber.StatusBadRequest, "invalid invite")
	}
	resp, err := s.CreateInvite(c.Context(), email, req.Role, &sess.AccountID)
	if err != nil {
		return err
	}
	s.audit(c, sess.AccountID, sess.DeviceID, "invite", map[string]string{"email": email, "role": req.Role})
	return c.JSON(resp)
}

func (s *Server) CreateInvite(ctx context.Context, email, role string, createdBy *string) (*proto.InviteResponse, error) {
	token := newToken()
	if err := s.store.CreateInvite(ctx, hashToken(token), email, role, createdBy, s.cfg.InviteTTL); err != nil {
		return nil, err
	}
	return &proto.InviteResponse{Token: token, URL: strings.TrimRight(s.cfg.PublicURL, "/") + "/invite/" + token}, nil
}

func (s *Server) listDevices(c fiber.Ctx) error {
	sess := sessionOf(c)
	devs, err := s.store.ListDevices(c.Context(), sess.AccountID)
	if err != nil {
		return err
	}
	out := make([]proto.Device, 0, len(devs))
	for _, d := range devs {
		out = append(out, proto.Device{ID: d.ID, Name: d.Name, Platform: d.Platform, CreatedAt: d.CreatedAt, LastSeenAt: d.LastSeenAt, Current: d.ID == sess.DeviceID})
	}
	return c.JSON(out)
}

func (s *Server) revokeDevice(c fiber.Ctx) error {
	sess := sessionOf(c)
	id := c.Params("id")
	if !validUUID(id) {
		return errNotFound
	}
	switch err := s.store.RevokeDevice(c.Context(), sess.AccountID, id); {
	case errors.Is(err, store.ErrNotFound):
		return errNotFound
	case err != nil:
		return err
	}
	s.audit(c, sess.AccountID, sess.DeviceID, "device_revoked", map[string]string{"device": id})
	return c.SendStatus(fiber.StatusNoContent)
}

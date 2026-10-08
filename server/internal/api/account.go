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

func (s *Server) handleGetAccount(c fiber.Ctx) error {
	account, err := s.store.GetAccountByID(c.Context(), getSession(c).AccountID)
	if err != nil {
		return err
	}
	var argon2Params crypt.Argon2Params
	if err := json.Unmarshal(account.Argon2Params, &argon2Params); err != nil {
		return err
	}
	return c.JSON(proto.Account{
		AccountID:  account.ID,
		Email:      account.Email,
		Name:       account.Name,
		Role:       account.Role,
		Argon2:     argon2Params,
		Salt:       account.Salt,
		EncKeyset:  account.EncKeyset,
		PublicKeys: getAllPublicKey(account),
	})
}

func getAllPublicKey(account *store.Account) crypt.PublicKeys {
	return crypt.PublicKeys{KEM: account.KEMPub, Sign: account.SignPub, KEMSig: account.KEMSig}
}

func (s *Server) handleListAllMember(c fiber.Ctx) error {
	allAccount, err := s.store.ListAllAccount(c.Context())
	if err != nil {
		return err
	}
	allMember := make([]proto.Member, 0, len(allAccount))
	for index := range allAccount {
		account := &allAccount[index]
		allMember = append(allMember, proto.Member{AccountID: account.ID, Email: account.Email, Name: account.Name, Role: account.Role, PublicKeys: getAllPublicKey(account)})
	}
	return c.JSON(allMember)
}

func (s *Server) handleCreateInvite(c fiber.Ctx) error {
	currentSession := getSession(c)
	if currentSession.Role != proto.RoleAdmin {
		return createAPIError(fiber.StatusForbidden, "admin only")
	}
	var request proto.InviteRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	email := normalizeEmail(request.Email)
	if !strings.Contains(email, "@") || (request.Role != proto.RoleAdmin && request.Role != proto.RoleMember) {
		return createAPIError(fiber.StatusBadRequest, "invalid invite")
	}
	response, err := s.CreateInvite(c.Context(), email, request.Role, &currentSession.AccountID)
	if err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "invite", map[string]string{"email": email, "role": request.Role})
	return c.JSON(response)
}

func (s *Server) CreateInvite(ctx context.Context, email, role string, createdBy *string) (*proto.InviteResponse, error) {
	token := generateToken()
	if err := s.store.CreateInvite(ctx, hashToken(token), email, role, createdBy, s.config.InviteTTL); err != nil {
		return nil, err
	}
	return &proto.InviteResponse{Token: token, URL: strings.TrimRight(s.config.PublicURL, "/") + "/invite/" + token}, nil
}

func (s *Server) handleListAllDevice(c fiber.Ctx) error {
	currentSession := getSession(c)
	allStoredDevice, err := s.store.ListAllDevice(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	allDevice := make([]proto.Device, 0, len(allStoredDevice))
	for _, device := range allStoredDevice {
		allDevice = append(allDevice, proto.Device{ID: device.ID, Name: device.Name, Platform: device.Platform, CreatedAt: device.CreatedAt, LastSeenAt: device.LastSeenAt, Current: device.ID == currentSession.DeviceID})
	}
	return c.JSON(allDevice)
}

func (s *Server) handleRevokeDevice(c fiber.Ctx) error {
	currentSession := getSession(c)
	deviceID := c.Params("id")
	if !isValidUUID(deviceID) {
		return errNotFound
	}
	switch err := s.store.RevokeDevice(c.Context(), currentSession.AccountID, deviceID); {
	case errors.Is(err, store.ErrNotFound):
		return errNotFound
	case err != nil:
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "device_revoked", map[string]string{"device": deviceID})
	return c.SendStatus(fiber.StatusNoContent)
}

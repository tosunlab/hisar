package api

import (
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
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
	return c.JSON(apitypes.Account{
		AccountID:  account.ID,
		Email:      account.Email,
		Name:       account.Name,
		Role:       account.Role,
		Argon2:     argon2Params,
		Salt:       account.Salt,
		EncKeyset:  account.EncKeyset,
		PublicKeys: getPublicKeys(account),
	})
}

func getPublicKeys(account *store.Account) crypt.PublicKeys {
	return crypt.PublicKeys{KEM: account.KEMPub, Sign: account.SignPub, KEMSig: account.KEMSig}
}

func (s *Server) handleListMembers(c fiber.Ctx) error {
	accounts, err := s.store.ListPeople(c.Context(), getSession(c).AccountID)
	if err != nil {
		return err
	}
	members := make([]apitypes.Member, 0, len(accounts))
	for index := range accounts {
		members = append(members, convertToProtoMember(&accounts[index]))
	}
	return c.JSON(members)
}

func (s *Server) handleListDevices(c fiber.Ctx) error {
	currentSession := getSession(c)
	storedDevices, err := s.store.ListDevices(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	devices := make([]apitypes.Device, 0, len(storedDevices))
	for _, device := range storedDevices {
		devices = append(devices, apitypes.Device{ID: device.ID, Name: device.Name, Platform: device.Platform, CreatedAt: device.CreatedAt, LastSeenAt: device.LastSeenAt, Current: device.ID == currentSession.DeviceID})
	}
	return c.JSON(devices)
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

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
		PublicKeys: getAllPublicKey(account),
	})
}

func getAllPublicKey(account *store.Account) crypt.PublicKeys {
	return crypt.PublicKeys{KEM: account.KEMPub, Sign: account.SignPub, KEMSig: account.KEMSig}
}

func (s *Server) handleListAllMember(c fiber.Ctx) error {
	allAccount, err := s.store.ListAllPerson(c.Context(), getSession(c).AccountID)
	if err != nil {
		return err
	}
	allMember := make([]apitypes.Member, 0, len(allAccount))
	for index := range allAccount {
		allMember = append(allMember, convertToProtoMember(&allAccount[index]))
	}
	return c.JSON(allMember)
}

func (s *Server) handleListAllDevice(c fiber.Ctx) error {
	currentSession := getSession(c)
	allStoredDevice, err := s.store.ListAllDevice(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	allDevice := make([]apitypes.Device, 0, len(allStoredDevice))
	for _, device := range allStoredDevice {
		allDevice = append(allDevice, apitypes.Device{ID: device.ID, Name: device.Name, Platform: device.Platform, CreatedAt: device.CreatedAt, LastSeenAt: device.LastSeenAt, Current: device.ID == currentSession.DeviceID})
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

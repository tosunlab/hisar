package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/server/internal/store"
)

func (s *Service) GetAccount(ctx context.Context, actor Actor) (apitypes.Account, error) {
	account, err := s.store.GetAccountByID(ctx, actor.AccountID)
	if err != nil {
		return apitypes.Account{}, err
	}
	var argon2Params crypt.Argon2Params
	if err := json.Unmarshal(account.Argon2Params, &argon2Params); err != nil {
		return apitypes.Account{}, err
	}
	return apitypes.Account{
		AccountID:  account.ID,
		Email:      account.Email,
		Name:       account.Name,
		Role:       account.Role,
		Argon2:     argon2Params,
		Salt:       account.Salt,
		EncKeyset:  account.EncKeyset,
		PublicKeys: getPublicKeys(account),
	}, nil
}

func getPublicKeys(account *store.Account) crypt.PublicKeys {
	return crypt.PublicKeys{KEM: account.KEMPub, Sign: account.SignPub, KEMSig: account.KEMSig}
}

func convertToProtoMember(account *store.Account) apitypes.Member {
	return apitypes.Member{AccountID: account.ID, Email: account.Email, Name: account.Name, Role: account.Role, PublicKeys: getPublicKeys(account)}
}

// ListMembers returns the people the caller shares a vault with, and the caller.
func (s *Service) ListMembers(ctx context.Context, actor Actor) ([]apitypes.Member, error) {
	accounts, err := s.store.ListPeople(ctx, actor.AccountID)
	if err != nil {
		return nil, err
	}
	members := make([]apitypes.Member, 0, len(accounts))
	for index := range accounts {
		members = append(members, convertToProtoMember(&accounts[index]))
	}
	return members, nil
}

func (s *Service) ListDevices(ctx context.Context, actor Actor) ([]apitypes.Device, error) {
	storedDevices, err := s.store.ListDevices(ctx, actor.AccountID)
	if err != nil {
		return nil, err
	}
	devices := make([]apitypes.Device, 0, len(storedDevices))
	for _, device := range storedDevices {
		devices = append(devices, apitypes.Device{ID: device.ID, Name: device.Name, Platform: device.Platform, CreatedAt: device.CreatedAt, LastSeenAt: device.LastSeenAt, Current: device.ID == actor.DeviceID})
	}
	return devices, nil
}

func (s *Service) RevokeDevice(ctx context.Context, actor Actor, deviceID string) error {
	if !IsValidUUID(deviceID) {
		return ErrNotFound
	}
	switch _, err := s.store.RevokeDevice(ctx, actor.AccountID, deviceID); {
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case err != nil:
		return err
	}
	s.recordAudit(ctx, actor, "device_revoked", map[string]string{"device": deviceID})
	return nil
}

package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/settings"
)

var (
	errEmailNotSetUp         = newError(KindInvalid, "Email is not set up. Save the SMTP settings first.")
	errEmailSetByEnvironment = newError(KindInvalid, "Email is set by HISAR_SMTP_* variables on the server, so it cannot be changed here.")
	errTooManyTestEmails     = newError(KindTooManyRequests, "Too many test emails. Wait a minute and try again.")
)

// GetSettings is the dashboard's Settings: every field's effective value and where it comes from. internal/api lets
// only admins reach it.
func (s *Service) GetSettings(ctx context.Context) (apitypes.AdminSettings, error) {
	stored, err := s.store.GetInstanceSettings(ctx)
	if err != nil {
		return apitypes.AdminSettings{}, err
	}
	effective, sources, err := settings.Resolve(s.instanceSettings.Defaults, stored, s.instanceSettings.Environment)
	if err != nil {
		return apitypes.AdminSettings{}, err
	}
	return describeSettings(effective, sources)
}

// UpdateSettings stores the changed fields, then swaps the snapshot every request reads. A field the environment sets
// is refused, any Email field once a HISAR_SMTP_* variable is set, and so is a result that fails Validate; each leaves
// the stored settings unchanged.
func (s *Service) UpdateSettings(ctx context.Context, actor Actor, request *apitypes.AdminSettingsUpdate) (apitypes.AdminSettings, error) {
	if len(request.Settings) == 0 {
		return apitypes.AdminSettings{}, newError(KindInvalid, "Change at least one setting.")
	}
	var changedFields []string
	effective, sources, err := s.updateStoredSettings(ctx, actor.AccountID, func(stored []byte) (updated []byte, err error) {
		updated, changedFields, err = s.applySettingChanges(stored, request.Settings)
		return updated, err
	})
	if err != nil {
		return apitypes.AdminSettings{}, err
	}
	s.recordSettingsChange(ctx, actor, changedFields)
	return describeSettings(effective, sources)
}

// applySettingChanges is what a save from Settings makes of the stored document, and the fields it changes.
func (s *Service) applySettingChanges(stored []byte, changes map[string]json.RawMessage) ([]byte, []string, error) {
	_, storedSources, err := settings.Resolve(s.instanceSettings.Defaults, stored, s.instanceSettings.Environment)
	if err != nil {
		return nil, nil, err
	}
	for _, field := range settings.ListFields() {
		if _, isChanged := changes[field.Name]; !isChanged || storedSources[field.Name] != settings.SourceEnvironment {
			continue
		}
		if field.IsEmail() {
			return nil, nil, errEmailSetByEnvironment
		}
		return nil, nil, newError(KindInvalid, fmt.Sprintf("%s is set by %s on the server, so it cannot be changed here.", field.Name, field.Environment))
	}
	updated, err := settings.Apply(stored, changes)
	if err != nil {
		return nil, nil, newError(KindInvalid, err.Error())
	}
	changedFields, err := settings.ListChangedFields(stored, updated)
	return updated, changedFields, err
}

func (s *Service) recordSettingsChange(ctx context.Context, actor Actor, changedFields []string) {
	if len(changedFields) > 0 {
		s.recordAudit(ctx, actor, "admin_settings_changed", map[string][]string{"fields": changedFields})
	}
}

// updateStoredSettings stores what update makes of the stored document, holding the row locked, then swaps the
// snapshot. A result that fails Validate leaves the stored settings unchanged.
func (s *Service) updateStoredSettings(ctx context.Context, updatedBy string, update func(stored []byte) ([]byte, error)) (settings.Settings, settings.Sources, error) {
	s.settingsMutex.Lock()
	defer s.settingsMutex.Unlock()
	var effective settings.Settings
	var sources settings.Sources
	err := s.store.UpdateInstanceSettings(ctx, updatedBy, func(stored []byte) ([]byte, error) {
		updated, err := update(stored)
		if err != nil {
			return nil, err
		}

		if effective, sources, err = settings.Resolve(s.instanceSettings.Defaults, updated, s.instanceSettings.Environment); err != nil {
			return nil, newError(KindInvalid, err.Error())
		}
		if err := effective.Validate(); err != nil {
			return nil, newError(KindInvalid, err.Error())
		}
		return updated, nil
	})
	if err != nil {
		return settings.Settings{}, nil, err
	}
	s.applySettings(effective)
	return effective, sources, nil
}

// applySettings swaps the snapshot every request reads for newly stored settings.
func (s *Service) applySettings(effective settings.Settings) {
	s.instanceSettings.Current.Store(&effective)
	if s.instanceSettings.OnSave != nil {
		s.instanceSettings.OnSave(effective)
	}
}

func describeSettings(effective settings.Settings, sources settings.Sources) (apitypes.AdminSettings, error) {
	values, err := effective.ListValues()
	if err != nil {
		return apitypes.AdminSettings{}, err
	}
	described := apitypes.AdminSettings{Settings: map[string]apitypes.AdminSetting{}}
	for _, field := range settings.ListFields() {
		value := values[field.Name]
		if field.IsSecret {
			if value, err = json.Marshal(apitypes.AdminSecretValue{IsSet: string(value) != `""`}); err != nil {
				return apitypes.AdminSettings{}, err
			}
		}
		described.Settings[field.Name] = apitypes.AdminSetting{Value: value, Source: string(sources[field.Name]), EnvironmentVariable: field.Environment}
	}
	return described, nil
}

// SendTestEmail sends one message with the current SMTP settings and answers the SMTP server's error when it fails. An
// admin may send a few at once, then five a minute.
func (s *Service) SendTestEmail(ctx context.Context, actor Actor, request *apitypes.AdminTestEmailRequest) error {
	if !s.Limiters.TestEmail.Allow(actor.AccountID) {
		return errTooManyTestEmails
	}
	recipient, err := parseEmail(request.To)
	if err != nil {
		return err
	}
	mailer := s.mailer()
	if mailer == nil {
		return errEmailNotSetUp
	}
	if err := mailer.Send(ctx, composeTestEmail(recipient, BuildPublicOrigin(s.config.PublicURL))); err != nil {
		return newError(KindUnavailable, "The test email was not sent: "+err.Error())
	}
	return nil
}

func composeTestEmail(recipient, origin string) mail.Message {
	return mail.Message{
		To:      recipient,
		Subject: "Test email from Hisar",
		Body: fmt.Sprintf(`This is a test email from the Hisar server at %s.
Email works: the server can send invites and sharing notifications.
`, origin),
	}
}

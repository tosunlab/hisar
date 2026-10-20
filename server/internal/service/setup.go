package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/settings"
)

var errSetupStep = newError(KindConflict, "This setup step is already done. Reload the page.")

// GetSetup is where setup of the server stands, which the setup page reads before anyone signs in.
func (s *Service) GetSetup() apitypes.Setup {
	return apitypes.Setup{State: string(s.currentSettings().SetupState)}
}

// PrepareSetup runs when the server starts. On a server that never stored a setup state, it starts setup when no account
// exists and skips it otherwise. While setup needs its first admin, it replaces any earlier setup invite with a fresh
// one and returns the setup page's URL, which holds the invite token after the "#"; otherwise it returns "".
func (s *Service) PrepareSetup(ctx context.Context) (string, error) {
	if err := s.store.DeleteUnusedSetupInvites(ctx); err != nil {
		return "", err
	}
	hasAccounts, err := s.store.HasAccounts(ctx)
	if err != nil {
		return "", err
	}
	stored, err := s.store.GetInstanceSettings(ctx)
	if err != nil {
		return "", err
	}
	isStored, err := isSetupStateStored(stored)
	if err != nil {
		return "", err
	}
	if !isStored {
		firstState := settings.SetupDone
		if !hasAccounts {
			firstState = settings.SetupNeedsAdmin
		}
		if _, _, err := s.updateStoredSettings(ctx, "", false, func(stored []byte) ([]byte, error) {
			if isStored, err := isSetupStateStored(stored); err != nil || isStored {
				return stored, err
			}
			return settings.SetStored(stored, setupStateKey, firstState)
		}); err != nil {
			return "", err
		}
	}
	if s.currentSettings().SetupState != settings.SetupNeedsAdmin {
		return "", nil
	}
	if hasAccounts {
		return "", s.moveSetup(ctx, "", false, settings.SetupNeedsAdmin, settings.SetupNeedsChecks)
	}
	token, _, err := s.newInviteToken(ctx, "", apitypes.RoleAdmin, nil)
	if err != nil {
		return "", err
	}
	return BuildPublicOrigin(s.config.PublicURL) + "/admin/setup#" + token, nil
}

// finishAdminStep moves setup on once the first account exists, whether it signed up on the setup page or with an
// invite from hisar-server invite, and deletes the setup invite so it cannot make a second admin.
func (s *Service) finishAdminStep(ctx context.Context, accountID string) error {
	if s.currentSettings().SetupState != settings.SetupNeedsAdmin {
		return nil
	}
	if err := s.moveSetup(ctx, accountID, false, settings.SetupNeedsAdmin, settings.SetupNeedsChecks); err != nil && !errors.Is(err, errSetupStep) {
		return err
	}
	return s.store.DeleteUnusedSetupInvites(ctx)
}

// FinishChecksStep runs the instance checks again and moves setup on when none failed; warnings can wait.
func (s *Service) FinishChecksStep(ctx context.Context, actor Actor, request CheckRequest) error {
	if s.currentSettings().SetupState != settings.SetupNeedsChecks {
		return errSetupStep
	}
	var failedTitles []string
	for _, check := range s.RunInstanceChecks(ctx, request) {
		if check.Status == apitypes.CheckStatusFailed {
			failedTitles = append(failedTitles, check.Title)
		}
	}
	if len(failedTitles) > 0 {
		return newError(KindConflict, fmt.Sprintf("Fix %s first, then check again.", strings.Join(failedTitles, ", ")))
	}
	return s.moveSetup(ctx, actor.AccountID, true, settings.SetupNeedsChecks, settings.SetupNeedsInvitePolicy)
}

// FinishInvitePolicyStep is the last setup step: it saves who can invite people the way Settings saves it, and marks
// setup done.
func (s *Service) FinishInvitePolicyStep(ctx context.Context, actor Actor, request *apitypes.SetupInvitePolicyRequest) error {
	if s.currentSettings().SetupState != settings.SetupNeedsInvitePolicy {
		return errSetupStep
	}
	invitePolicy, err := json.Marshal(request.InvitePolicy)
	if err != nil {
		return err
	}
	var changedFields []string
	if _, _, err := s.updateStoredSettings(ctx, actor.AccountID, true, func(stored []byte) ([]byte, error) {
		if err := s.requireSetupState(stored, settings.SetupNeedsInvitePolicy); err != nil {
			return nil, err
		}
		updated, changed, err := s.applySettingChanges(stored, map[string]json.RawMessage{"invitePolicy": invitePolicy})
		if err != nil {
			return nil, err
		}
		changedFields = changed
		return settings.SetStored(updated, setupStateKey, settings.SetupDone)
	}); err != nil {
		return err
	}
	s.recordSettingsChange(ctx, actor, changedFields)
	s.recordAudit(ctx, actor, auditSetupCompleted, nil)
	return nil
}

// moveSetup moves setup from one step to the next, and refuses with errSetupStep when setup is at another step.
func (s *Service) moveSetup(ctx context.Context, updatedBy string, isByAdmin bool, from, to settings.SetupState) error {
	_, _, err := s.updateStoredSettings(ctx, updatedBy, isByAdmin, func(stored []byte) ([]byte, error) {
		if err := s.requireSetupState(stored, from); err != nil {
			return nil, err
		}
		return settings.SetStored(stored, setupStateKey, to)
	})
	return err
}

const setupStateKey = "setupState"

func (s *Service) requireSetupState(stored []byte, state settings.SetupState) error {
	effective, _, err := settings.Resolve(s.instanceSettings.Defaults, stored, nil)
	if err != nil {
		return err
	}
	if effective.SetupState != state {
		return errSetupStep
	}
	return nil
}

func isSetupStateStored(stored []byte) (bool, error) {
	if len(stored) == 0 {
		return false, nil
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(stored, &document); err != nil {
		return false, fmt.Errorf("setup: stored settings: %w", err)
	}
	_, isStored := document[setupStateKey]
	return isStored, nil
}

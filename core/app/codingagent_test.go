package app

import (
	"errors"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/localstate"
)

func TestVaultAvailableToAgentsIsSavedAndOffByDefault(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	if vaultIDs := service.ListAgentVaultIDs(); len(vaultIDs) != 0 {
		t.Fatalf("available by default: %q", vaultIDs)
	}
	if vaultIDs, err := service.SetVaultAvailableToAgents(testService.personalID, true); err != nil || !slices.Equal(vaultIDs, []string{testService.personalID}) {
		t.Fatalf("turn on: %q %v", vaultIDs, err)
	}
	service.SetVaultAvailableToAgents(testService.personalID, true)
	service.SetVaultAvailableToAgents(testService.familyID, true)
	if vaultIDs, _ := service.SetVaultAvailableToAgents(testService.familyID, false); !slices.Equal(vaultIDs, []string{testService.personalID}) {
		t.Errorf("turn off: %q", vaultIDs)
	}
	state, err := localstate.Load(service.config.StatePath)
	if err != nil || !slices.Equal(state.AgentVaultIDs, []string{testService.personalID}) {
		t.Errorf("saved %q %v", state.AgentVaultIDs, err)
	}
	if _, err := service.SetVaultAvailableToAgents("no-such-vault", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown vault: %v", err)
	}
	service.Lock()
	if _, err := service.SetVaultAvailableToAgents(testService.personalID, false); !errors.Is(err, ErrLocked) {
		t.Errorf("locked: %v", err)
	}
	if vaultIDs := service.ListAgentVaultIDs(); !slices.Equal(vaultIDs, []string{testService.personalID}) {
		t.Errorf("after lock: %q", vaultIDs)
	}
}

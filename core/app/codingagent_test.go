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
	if allVaultID := service.ListAllAgentVaultID(); len(allVaultID) != 0 {
		t.Fatalf("available by default: %q", allVaultID)
	}
	if allVaultID, err := service.SetVaultAvailableToAgents(testService.personalID, true); err != nil || !slices.Equal(allVaultID, []string{testService.personalID}) {
		t.Fatalf("turn on: %q %v", allVaultID, err)
	}
	service.SetVaultAvailableToAgents(testService.personalID, true)
	service.SetVaultAvailableToAgents(testService.familyID, true)
	if allVaultID, _ := service.SetVaultAvailableToAgents(testService.familyID, false); !slices.Equal(allVaultID, []string{testService.personalID}) {
		t.Errorf("turn off: %q", allVaultID)
	}
	state, err := localstate.Load(service.config.StatePath)
	if err != nil || !slices.Equal(state.AllAgentVaultID, []string{testService.personalID}) {
		t.Errorf("saved %q %v", state.AllAgentVaultID, err)
	}
	if _, err := service.SetVaultAvailableToAgents("no-such-vault", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown vault: %v", err)
	}
	service.Lock()
	if _, err := service.SetVaultAvailableToAgents(testService.personalID, false); !errors.Is(err, ErrLocked) {
		t.Errorf("locked: %v", err)
	}
	if allVaultID := service.ListAllAgentVaultID(); !slices.Equal(allVaultID, []string{testService.personalID}) {
		t.Errorf("after lock: %q", allVaultID)
	}
}

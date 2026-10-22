package store

import (
	"errors"
	"testing"
)

func TestGenerationZeroOnlyBeforeFirstRotation(t *testing.T) {
	for _, organizationID := range []string{"", "organization"} {
		for _, generation := range []int64{1, 2, 3} {
			vault := &lockedVault{organizationID: organizationID, keyGeneration: generation}
			wantLegacy := organizationID == "" && generation == 1
			if got := vault.acceptsKeyGeneration(0); got != wantLegacy {
				t.Errorf("organization %q generation %d accepts omitted generation = %v, want %v", organizationID, generation, got, wantLegacy)
			}
			if !vault.acceptsKeyGeneration(generation) || vault.acceptsKeyGeneration(generation+1) {
				t.Errorf("generation %d did not require its current explicit generation", generation)
			}
			if !wantLegacy && !errors.Is(vault.checkItemKeyGeneration(0), ErrKeyGenerationMissing) {
				t.Errorf("generation %d must tell legacy writers to update", generation)
			}
		}
	}
}

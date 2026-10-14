package session

import (
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
)

func TestGetVaultPermissions(t *testing.T) {
	cases := []struct {
		name                   string
		vaultType, role        string
		vaultCount, ownerCount int
		want                   VaultPermissions
	}{
		{"personal", apitypes.VaultPersonal, apitypes.VaultOwner, 3, 1, VaultPermissions{CanRename: true, DeleteBlockedReason: PersonalVaultDeleteReason, LeaveBlockedReason: PersonalVaultLeaveReason}},
		{"only owner", apitypes.VaultShared, apitypes.VaultOwner, 3, 1, VaultPermissions{CanRename: true, CanDelete: true, LeaveBlockedReason: OnlyOwnerLeaveReason}},
		{"one of two owners", apitypes.VaultShared, apitypes.VaultOwner, 3, 2, VaultPermissions{CanRename: true, CanDelete: true, CanLeave: true}},
		{"only vault", apitypes.VaultShared, apitypes.VaultOwner, 1, 2, VaultPermissions{CanRename: true, DeleteBlockedReason: OnlyVaultDeleteReason, CanLeave: true}},
		{"editor", apitypes.VaultShared, apitypes.VaultEditor, 3, 1, VaultPermissions{DeleteBlockedReason: NotOwnerDeleteReason, CanLeave: true}},
	}
	for _, testCase := range cases {
		if got := GetVaultPermissions(testCase.vaultType, testCase.role, testCase.vaultCount, testCase.ownerCount); got != testCase.want {
			t.Errorf("%s: got %+v, want %+v", testCase.name, got, testCase.want)
		}
	}
}

package service

import (
	"fmt"
	"testing"

	"github.com/tryhisar/hisar/server/internal/store"
)

func TestBusyAccountDeletionIsRetryable(t *testing.T) {
	for _, err := range []error{store.ErrDeletionBusy, fmt.Errorf("deletion: %w", store.ErrDeletionBusy)} {
		converted := convertAdminAccountError(err)
		if converted != errDeletionBusy || errDeletionBusy.Kind != KindConflict {
			t.Errorf("busy deletion must return the retryable conflict, got %v", converted)
		}
	}
	if errDeletionBusy.Error() != "The account's memberships are changing. Try deleting the account again." {
		t.Errorf("admin deletion refusal: %v", errDeletionBusy)
	}
}

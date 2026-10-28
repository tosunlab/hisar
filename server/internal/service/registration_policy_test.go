package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/server/internal/settings"
)

func TestRegistrationPolicyMatchesCheckInvite(t *testing.T) {
	var snapshot atomic.Pointer[settings.Settings]
	service := &Service{instanceSettings: InstanceSettings{Current: &snapshot}}
	for _, state := range []settings.SetupState{settings.SetupNeedsAdmin, settings.SetupNeedsChecks, settings.SetupNeedsInvitePolicy, settings.SetupDone} {
		for _, open := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/open=%t", state, open), func(t *testing.T) {
				current := settings.Defaults()
				current.SetupState, current.OpenRegistration = state, open
				snapshot.Store(&current)
				expected := "invite"
				if state == settings.SetupNeedsAdmin {
					expected = "setup"
				} else if open {
					expected = "open"
				}
				policy := service.GetRegistrationPolicy()
				if policy.Mode != expected {
					t.Fatalf("mode = %q, want %q", policy.Mode, expected)
				}
				err := service.checkInvite(context.Background(), "", "ada@example.com")
				if expected == "open" {
					if err != nil {
						t.Fatalf("open policy refused an empty invite: %v", err)
					}
				} else if !errors.Is(err, errSignUpClosed) {
					t.Fatalf("%s policy did not refuse an empty invite: %v", expected, err)
				}
			})
		}
	}
}

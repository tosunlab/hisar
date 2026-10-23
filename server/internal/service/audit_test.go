package service

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
)

func TestAuditEventThatCannotBeRecordedIsLogged(t *testing.T) {
	service := createSettingsService(t, nil)
	var logged bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(previousWriter) })
	service.recordAudit(context.Background(), Actor{IP: "192.0.2.1"}, auditLogin, map[string]any{"device": make(chan int)})
	if !strings.Contains(logged.String(), "audit event login not recorded") {
		t.Errorf("log: %q", logged.String())
	}
}

func TestEveryAuditTypeHasALabel(t *testing.T) {
	groupIDs := map[string]bool{}
	for _, group := range auditGroups {
		groupIDs[group.id] = true
	}
	names := map[string]bool{}
	for eventType, row := range auditTypes {
		if row.name == "" || row.label == "" || !groupIDs[row.group] {
			t.Errorf("audit type %d has no name, label or known group: %+v", eventType, row)
		}
		if names[row.name] {
			t.Errorf("audit type %q is in the table twice", row.name)
		}
		names[row.name] = true
	}
	for _, name := range []string{
		"signup", "login", "account_deleted", "share-link-create", "team_vault_revoked", "admin_settings_changed",
		"admin_secrets_backup_confirmed", "setup_completed", "operator_invite", "login_refused_suspended", "admin_account_suspended", "admin_account_reactivated",
		"admin_account_deleted", "admin_role_changed", "admin_signed_out", "admin_device_revoked", "admin_invite_created",
		"admin_invite_revoked", "admin_invite_resent", "admin_audit_exported",
	} {
		if !names[name] {
			t.Errorf("%s is missing", name)
		}
	}
}

package settings

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func readEnvironment(variables map[string]string) Environment {
	return func(name string) string { return variables[name] }
}

func TestResolvePrecedence(t *testing.T) {
	stored := []byte(`{"smtp":{"host":"stored.example.com","port":2525},"invitePolicy":"admins","authBurst":20}`)
	environment := readEnvironment(map[string]string{"HISAR_SMTP_HOST": " env.example.com ", "HISAR_AUTH_BURST": "30"})
	effective, sources, err := Resolve(Defaults(), stored, environment)
	if err != nil {
		t.Fatal(err)
	}
	for name, expected := range map[string]Source{
		"smtp.host":           SourceEnvironment,
		"authBurst":           SourceEnvironment,
		"smtp.port":           SourceEnvironment,
		"smtp.from":           SourceEnvironment,
		"invitePolicy":        SourceDatabase,
		"sessionLifetimeDays": SourceDefault,
	} {
		if sources[name] != expected {
			t.Errorf("%s: source %s, want %s", name, sources[name], expected)
		}
	}
	if effective.SMTP.Host != "env.example.com" || effective.AuthBurst != 30 || effective.SMTP.Port != 587 || effective.InvitePolicy != InvitePolicyAdmins ||
		effective.SessionLifetime != 30 {
		t.Errorf("effective %+v", effective)
	}
	if len(sources) != len(ListFields()) {
		t.Errorf("%d sources for %d fields", len(sources), len(ListFields()))
	}
}

func TestResolveLetsAnySMTPVariableOwnEmail(t *testing.T) {
	stored := []byte(`{"smtp":{"host":"stored.example.com","port":2525,"username":"stored","password":"stored secret","from":"stored@example.com"}}`)
	for _, variable := range []string{"HISAR_SMTP_HOST", "HISAR_SMTP_PORT", "HISAR_SMTP_USERNAME", "HISAR_SMTP_PASSWORD", "HISAR_SMTP_FROM"} {
		effective, sources, err := Resolve(Defaults(), stored, readEnvironment(map[string]string{variable: "25"}))
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range ListFields() {
			if field.IsEmail() && sources[field.Name] != SourceEnvironment {
				t.Errorf("%s: %s comes from %s", variable, field.Name, sources[field.Name])
			}
		}
		if strings.Contains(fmt.Sprint(effective.SMTP), "stored") || effective.SMTP.Port == 2525 {
			t.Errorf("%s: the stored SMTP settings were used: %+v", variable, effective.SMTP)
		}
	}
	if effective, sources, err := Resolve(Defaults(), stored, readEnvironment(map[string]string{"HISAR_SMTP_HOST": " ", "HISAR_AUTH_BURST": "5"})); err != nil ||
		effective.SMTP.Password != "stored secret" || sources["smtp.host"] != SourceDatabase {
		t.Errorf("a blank SMTP variable took Email: %+v %v %v", effective.SMTP, sources, err)
	}
}

func TestResolveKeepsDefaultsForMissingFields(t *testing.T) {
	effective, sources, err := Resolve(Defaults(), []byte(`{"retention":{"auditDays":30},"smtp":{"username":"hisar"}}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	expected := Defaults()
	expected.Retention.Audit = 30
	expected.SMTP.Username = "hisar"
	if effective != expected {
		t.Errorf("effective %+v, want %+v", effective, expected)
	}
	if sources["retention.auditDays"] != SourceDatabase || sources["retention.historyDays"] != SourceDefault || sources["smtp.port"] != SourceDefault {
		t.Errorf("sources %v", sources)
	}
	for _, stored := range [][]byte{nil, []byte(`{}`), []byte(`{"retiredSetting":true}`)} {
		if effective, _, err := Resolve(Defaults(), stored, nil); err != nil || effective != Defaults() {
			t.Errorf("%s: %+v %v", stored, effective, err)
		}
	}
}

func TestResolveReadsTheEnvironment(t *testing.T) {
	effective, _, err := Resolve(Defaults(), nil, readEnvironment(map[string]string{
		"HISAR_SMTP_HOST": "smtp.example.com", "HISAR_SMTP_PORT": "465", "HISAR_SMTP_USERNAME": "hisar", "HISAR_SMTP_PASSWORD": " secret ",
		"HISAR_SMTP_FROM": "Hisar <noreply@tryhisar.com>", "HISAR_MINIMUM_VERSION_DESKTOP_MACOS": " 0.1.6 ", "HISAR_MINIMUM_VERSION_IOS": "",
		"HISAR_MINIMUM_VERSION_CLI": "1.0.0-alpha-test", "HISAR_MINIMUM_VERSION_EXTENSION": "0.2.0", "HISAR_MINIMUM_VERSION_ANDROID": "1.1.0",
	}))
	if err != nil {
		t.Fatal(err)
	}
	expected := SMTP{Host: "smtp.example.com", Port: 465, Username: "hisar", Password: " secret ", From: "Hisar <noreply@tryhisar.com>"}
	if effective.SMTP != expected {
		t.Errorf("SMTP %+v", effective.SMTP)
	}
	if effective.MinimumVersions != (MinimumVersions{DesktopMacOS: "0.1.6", Android: "1.1.0", CLI: "1.0.0-alpha-test", Extension: "0.2.0"}) {
		t.Errorf("minimum versions %+v", effective.MinimumVersions)
	}
	if err := effective.Validate(); err != nil {
		t.Error(err)
	}
	for _, name := range []string{"HISAR_SMTP_PORT", "HISAR_AUTH_BURST"} {
		if _, _, err := Resolve(Defaults(), nil, readEnvironment(map[string]string{name: "submission"})); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s accepted a name: %v", name, err)
		}
	}
}

func TestResolveRefusesACorruptDocument(t *testing.T) {
	for _, stored := range []string{`not json`, `{"smtp":{"port":"587"}}`, `[]`} {
		if _, _, err := Resolve(Defaults(), []byte(stored), nil); err == nil {
			t.Errorf("%s accepted", stored)
		}
	}
}

func TestValidateBounds(t *testing.T) {
	if err := Defaults().Validate(); err != nil {
		t.Fatalf("defaults: %v", err)
	}
	withSMTP := func(change func(*SMTP)) func(*Settings) {
		return func(settings *Settings) {
			settings.SMTP = SMTP{Host: "smtp.example.com", Port: 587, Username: "hisar", Password: "secret", From: "noreply@example.com"}
			change(&settings.SMTP)
		}
	}
	for name, testCase := range map[string]struct {
		change  func(*Settings)
		isValid bool
	}{
		"SMTP set":                       {withSMTP(func(*SMTP) {}), true},
		"SMTP without a username":        {withSMTP(func(smtp *SMTP) { smtp.Username, smtp.Password = "", "" }), true},
		"SMTP port 1":                    {withSMTP(func(smtp *SMTP) { smtp.Port = 1 }), true},
		"SMTP port 65535":                {withSMTP(func(smtp *SMTP) { smtp.Port = 65535 }), true},
		"SMTP port 0":                    {withSMTP(func(smtp *SMTP) { smtp.Port = 0 }), false},
		"SMTP port 65536":                {withSMTP(func(smtp *SMTP) { smtp.Port = 65536 }), false},
		"SMTP sender not an address":     {withSMTP(func(smtp *SMTP) { smtp.From = "not an address" }), false},
		"SMTP username without password": {withSMTP(func(smtp *SMTP) { smtp.Password = "" }), false},
		"SMTP off ignores the other":     {func(settings *Settings) { settings.SMTP = SMTP{Port: 0, From: "x"} }, true},
		"minimum versions": {func(settings *Settings) {
			settings.MinimumVersions = MinimumVersions{DesktopMacOS: "0.1.6", Extension: "1.0.0-alpha-test"}
		}, true},
		"minimum macOS 0.1":                 {func(settings *Settings) { settings.MinimumVersions.DesktopMacOS = "0.1" }, false},
		"minimum iOS latest":                {func(settings *Settings) { settings.MinimumVersions.IOS = "latest" }, false},
		"minimum Android 1":                 {func(settings *Settings) { settings.MinimumVersions.Android = "1" }, false},
		"minimum CLI v1.0.0":                {func(settings *Settings) { settings.MinimumVersions.CLI = "v1.0.0" }, false},
		"minimum extension 1.0.0-":          {func(settings *Settings) { settings.MinimumVersions.Extension = "1.0.0-" }, false},
		"invite policy admins":              {func(settings *Settings) { settings.InvitePolicy = InvitePolicyAdmins }, true},
		"invite policy empty":               {func(settings *Settings) { settings.InvitePolicy = "" }, false},
		"invite policy unknown":             {func(settings *Settings) { settings.InvitePolicy = "owners" }, false},
		"session lifetime 1":                {func(settings *Settings) { settings.SessionLifetime = 1 }, true},
		"session lifetime 365":              {func(settings *Settings) { settings.SessionLifetime = 365 }, true},
		"session lifetime 0":                {func(settings *Settings) { settings.SessionLifetime = 0 }, false},
		"session lifetime 366":              {func(settings *Settings) { settings.SessionLifetime = 366 }, false},
		"auth burst 1":                      {func(settings *Settings) { settings.AuthBurst = 1 }, true},
		"auth burst 0":                      {func(settings *Settings) { settings.AuthBurst = 0 }, false},
		"history 1":                         {func(settings *Settings) { settings.Retention.History = 1 }, true},
		"history 3650":                      {func(settings *Settings) { settings.Retention.History = 3650 }, true},
		"history 0":                         {func(settings *Settings) { settings.Retention.History = 0 }, false},
		"history 3651":                      {func(settings *Settings) { settings.Retention.History = 3651 }, false},
		"audit 29":                          {func(settings *Settings) { settings.Retention.Audit = 29 }, false},
		"audit 30":                          {func(settings *Settings) { settings.Retention.Audit = 30 }, true},
		"audit 3650":                        {func(settings *Settings) { settings.Retention.Audit = 3650 }, true},
		"audit 3651":                        {func(settings *Settings) { settings.Retention.Audit = 3651 }, false},
		"expired invites 0":                 {func(settings *Settings) { settings.Retention.ExpiredInvites = 0 }, false},
		"expired invites 1":                 {func(settings *Settings) { settings.Retention.ExpiredInvites = 1 }, true},
		"expired invites 3651":              {func(settings *Settings) { settings.Retention.ExpiredInvites = 3651 }, false},
		"expired invites 3650 and audit 30": {func(settings *Settings) { settings.Retention.ExpiredInvites, settings.Retention.Audit = 3650, 30 }, true},
	} {
		settings := Defaults()
		testCase.change(&settings)
		if err := settings.Validate(); (err == nil) != testCase.isValid {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestApply(t *testing.T) {
	stored := []byte(`{"smtp":{"host":"old.example.com","password":"secret"},"futureField":true}`)
	updated, err := Apply(stored, map[string]json.RawMessage{
		"smtp.host": json.RawMessage(`"new.example.com"`), "smtp.port": json.RawMessage(`2525`), "retention.auditDays": json.RawMessage(`30`),
	})
	if err != nil {
		t.Fatal(err)
	}
	effective, sources, err := Resolve(Defaults(), updated, nil)
	if err != nil {
		t.Fatal(err)
	}
	if effective.SMTP.Host != "new.example.com" || effective.SMTP.Port != 2525 || effective.SMTP.Password != "" || effective.Retention.Audit != 30 {
		t.Errorf("effective %+v", effective)
	}
	if sources["retention.auditDays"] != SourceDatabase || sources["retention.historyDays"] != SourceDefault {
		t.Errorf("sources %v", sources)
	}
	if trimmed, err := Apply(nil, map[string]json.RawMessage{"smtp.host": json.RawMessage(`" smtp.example.com "`), "smtp.password": json.RawMessage(`" secret "`)}); err != nil ||
		!strings.Contains(string(trimmed), `"host":"smtp.example.com"`) || !strings.Contains(string(trimmed), `"password":" secret "`) {
		t.Errorf("trimmed %s %v", trimmed, err)
	}
	if !strings.Contains(string(updated), `"futureField":true`) {
		t.Errorf("a field this version does not know was dropped: %s", updated)
	}
	changed, err := ListChangedFields(stored, updated)
	if err != nil || !slices.Equal(changed, []string{"smtp.host", "smtp.port", "smtp.password", "retention.auditDays"}) {
		t.Errorf("changed %v %v", changed, err)
	}
	if changed, err := ListChangedFields(updated, updated); err != nil || len(changed) != 0 {
		t.Errorf("unchanged %v %v", changed, err)
	}

	for name, value := range map[string]string{
		"smtp.port":    `"587"`,
		"smtp.host":    `25`,
		"authBurst":    `2.5`,
		"invitePolicy": `null`,
		"smtp.nothing": `"x"`,
		"smtp":         `{}`,
	} {
		if _, err := Apply(stored, map[string]json.RawMessage{name: json.RawMessage(value)}); err == nil {
			t.Errorf("%s = %s accepted", name, value)
		}
	}
}

func TestApplyClearsThePasswordWhenTheServerChanges(t *testing.T) {
	stored := []byte(`{"smtp":{"host":"smtp.example.com","username":"hisar","password":"secret","from":"noreply@example.com"}}`)
	for name, testCase := range map[string]struct {
		changes          map[string]string
		expectedPassword string
	}{
		"host":                       {map[string]string{"smtp.host": `"attacker.example.com"`}, ""},
		"port":                       {map[string]string{"smtp.port": `2525`}, ""},
		"username":                   {map[string]string{"smtp.username": `"other"`}, ""},
		"host with the password":     {map[string]string{"smtp.host": `"new.example.com"`, "smtp.password": `"new secret"`}, "new secret"},
		"sender":                     {map[string]string{"smtp.from": `"other@example.com"`}, "secret"},
		"the default port again":     {map[string]string{"smtp.port": `587`}, "secret"},
		"the same host, other space": {map[string]string{"smtp.host": `" smtp.example.com "`}, "secret"},
		"another group":              {map[string]string{"authBurst": `20`}, "secret"},
	} {
		changes := map[string]json.RawMessage{}
		for field, value := range testCase.changes {
			changes[field] = json.RawMessage(value)
		}
		updated, err := Apply(stored, changes)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		effective, _, err := Resolve(Defaults(), updated, nil)
		if err != nil || effective.SMTP.Password != testCase.expectedPassword {
			t.Errorf("%s: password %q, want %q (%v)", name, effective.SMTP.Password, testCase.expectedPassword, err)
		}
	}
	if updated, err := Apply(nil, map[string]json.RawMessage{"smtp.host": json.RawMessage(`"smtp.example.com"`)}); err != nil || strings.Contains(string(updated), "password") {
		t.Errorf("a password was stored where none was: %s %v", updated, err)
	}
}

func TestSetStoredRecordsTheSecretsBackup(t *testing.T) {
	confirmedAt := time.Date(2026, 10, 17, 12, 30, 0, 0, time.UTC)
	stored := []byte(`{"smtp":{"host":"smtp.example.com"},"futureField":true}`)
	updated, err := SetStored(stored, "secretsBackupConfirmedAt", confirmedAt)
	if err != nil {
		t.Fatal(err)
	}
	effective, sources, err := Resolve(Defaults(), updated, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !effective.SecretsBackupConfirmedAt.Equal(confirmedAt) || effective.SMTP.Host != "smtp.example.com" || !strings.Contains(string(updated), `"futureField":true`) {
		t.Errorf("effective %+v from %s", effective, updated)
	}
	if _, isListed := sources["secretsBackupConfirmedAt"]; isListed {
		t.Error("the backup confirmation is listed as a setting")
	}
	if values, err := effective.ListValues(); err != nil || len(values) != len(ListFields()) {
		t.Errorf("%d values: %v", len(values), err)
	}
	if changed, err := ListChangedFields(stored, updated); err != nil || len(changed) != 0 {
		t.Errorf("changed %v %v", changed, err)
	}
	if _, err := Apply(updated, map[string]json.RawMessage{"secretsBackupConfirmedAt": json.RawMessage(`"2020-01-01T00:00:00Z"`)}); err == nil {
		t.Error("Apply set the backup confirmation")
	}
	if defaults, err := json.Marshal(Defaults()); err != nil || strings.Contains(string(defaults), "secretsBackupConfirmedAt") {
		t.Errorf("defaults %s %v", defaults, err)
	}
}

func TestListValues(t *testing.T) {
	effective := Defaults()
	effective.SMTP.Password = "secret"
	values, err := effective.ListValues()
	if err != nil || len(values) != len(ListFields()) {
		t.Fatalf("%d values: %v", len(values), err)
	}
	for name, expected := range map[string]string{"smtp.password": `"secret"`, "smtp.port": `587`, "retention.auditDays": `365`, "minimumVersions.iOS": `""`} {
		if string(values[name]) != expected {
			t.Errorf("%s = %s, want %s", name, values[name], expected)
		}
	}
}

func TestInvitePolicyAllowsInvite(t *testing.T) {
	if !InvitePolicyAnyone.AllowsInvite("member") || !InvitePolicyAdmins.AllowsInvite("admin") || InvitePolicyAdmins.AllowsInvite("member") {
		t.Error("invite policy")
	}
}

func TestOpenRegistration(t *testing.T) {
	if Defaults().OpenRegistration {
		t.Error("registration is open by default")
	}
	for text, expected := range map[string]bool{"true": true, "1": true, "false": false, " TRUE ": true} {
		effective, sources, err := Resolve(Defaults(), []byte(`{"openRegistration":true}`), readEnvironment(map[string]string{"HISAR_OPEN_REGISTRATION": text}))
		if err != nil || effective.OpenRegistration != expected || sources["openRegistration"] != SourceEnvironment {
			t.Errorf("HISAR_OPEN_REGISTRATION=%q: %v %v %v", text, effective.OpenRegistration, sources["openRegistration"], err)
		}
	}
	if _, _, err := Resolve(Defaults(), nil, readEnvironment(map[string]string{"HISAR_OPEN_REGISTRATION": "yes"})); err == nil || !strings.Contains(err.Error(), "HISAR_OPEN_REGISTRATION") {
		t.Errorf("HISAR_OPEN_REGISTRATION=yes accepted: %v", err)
	}
	updated, err := Apply(nil, map[string]json.RawMessage{"openRegistration": json.RawMessage(`true`)})
	if err != nil {
		t.Fatal(err)
	}
	if effective, sources, err := Resolve(Defaults(), updated, nil); err != nil || !effective.OpenRegistration || sources["openRegistration"] != SourceDatabase {
		t.Errorf("stored open registration: %+v %v %v", effective, sources, err)
	}
	for _, value := range []string{`"true"`, `1`, `null`} {
		if _, err := Apply(nil, map[string]json.RawMessage{"openRegistration": json.RawMessage(value)}); err == nil || !strings.Contains(err.Error(), "true or false") {
			t.Errorf("openRegistration = %s: %v", value, err)
		}
	}
}

func TestPushRelayDefaultsToHisarsAndTurnsOff(t *testing.T) {
	effective, _, err := Resolve(Defaults(), nil, readEnvironment(nil))
	if err != nil || effective.Push.GetRelayURL() != DefaultPushRelayURL {
		t.Fatalf("default relay = %q %v", effective.Push.GetRelayURL(), err)
	}
	effective, sources, err := Resolve(Defaults(), nil, readEnvironment(map[string]string{"HISAR_PUSH_RELAY_URL": "off"}))
	if err != nil || effective.Push.GetRelayURL() != "" || sources["push.relayURL"] != SourceEnvironment || effective.Validate() != nil {
		t.Fatalf("off = %q %v %v", effective.Push.GetRelayURL(), sources["push.relayURL"], err)
	}
	effective, _, _ = Resolve(Defaults(), []byte(`{"push":{"relayURL":""}}`), readEnvironment(nil))
	if effective.Push.GetRelayURL() != "" || effective.Validate() != nil {
		t.Fatalf("an empty relay from the dashboard = %q", effective.Push.GetRelayURL())
	}
	for relayURL, isValid := range map[string]bool{
		"https://push.example.com/": true, "http://127.0.0.1:8090": true, "http://push.example.com": false,
		"https://user@push.example.com": false, "push.example.com": false,
	} {
		settings := Defaults()
		settings.Push.RelayURL = relayURL
		if err := settings.Validate(); (err == nil) != isValid {
			t.Errorf("%s: %v", relayURL, err)
		}
	}
}

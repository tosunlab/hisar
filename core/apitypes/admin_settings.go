package apitypes

import "encoding/json"

// AdminSettings is GET and PUT /api/v1/admin/settings: every instance setting the dashboard edits, by name, such as
// "smtp.host" or "retention.auditDays".
type AdminSettings struct {
	Settings map[string]AdminSetting `json:"settings"`
}

// AdminSetting is one setting's effective value and its source: "default", "database" or "environment", which the
// dashboard cannot change. A secret's value is an AdminSecretValue.
type AdminSetting struct {
	Value  json.RawMessage `json:"value"`
	Source string          `json:"source"`
	// EnvironmentVariable is the HISAR_* variable that overrides the setting, if any.
	EnvironmentVariable string `json:"environmentVariable,omitempty"`
}

// AdminSecretValue stands for a secret setting, such as the SMTP password, which the server never sends back.
type AdminSecretValue struct {
	IsSet bool `json:"set"`
}

// AdminSettingsUpdate is the body of PUT /api/v1/admin/settings: the settings to store, by name; others keep their value.
type AdminSettingsUpdate struct {
	Settings map[string]json.RawMessage `json:"settings"`
}

// AdminTestEmailRequest is the body of POST /api/v1/admin/settings/test-email.
type AdminTestEmailRequest struct {
	To string `json:"to"`
}

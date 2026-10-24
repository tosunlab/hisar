// Package settings is the server's instance settings: built-in defaults, the document an admin saves from the dashboard,
// and the HISAR_* variables that override both, field by field except for Email, which they override as a whole.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	netmail "net/mail"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/appversion"
)

// Settings are the effective instance settings. Optional fields keep their defaults.
// Store encrypts the SMTP password when persisting this document.
type Settings struct {
	SMTP            SMTP            `json:"smtp"`
	MinimumVersions MinimumVersions `json:"minimumVersions"`
	InvitePolicy    InvitePolicy    `json:"invitePolicy"`
	SessionLifetime Days            `json:"sessionLifetimeDays"`
	// AuthBurst is how many sign-in requests one IP address may make before it is limited to one every 6 seconds.
	AuthBurst int       `json:"authBurst"`
	Retention Retention `json:"retention"`
	Push      Push      `json:"push"`
	// SecretsBackupConfirmedAt is when an admin confirmed on Overview that hisar-secrets.json is backed up; zero until
	// then. It is not in ListFields: the server records it, and Settings neither shows nor edits it.
	SecretsBackupConfirmedAt time.Time `json:"secretsBackupConfirmedAt,omitzero"`
	// SetupState is how far the first admin got in setting up the server from the dashboard. The dashboard cannot
	// edit it: each setup step moves it on.
	SetupState SetupState `json:"setupState"`
}

// SMTP is how the server sends email; an empty Host turns email off.
type SMTP struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
}

func (smtp SMTP) IsSet() bool { return smtp.Host != "" }

// DefaultPushRelayURL is Hisar's own push relay, which delivers to the official iPhone and Android apps for any server.
const DefaultPushRelayURL = "https://push.tryhisar.com"

// Push is where the server sends the phone apps' push notifications. An empty relay URL, or "off" from the
// environment, turns them off.
type Push struct {
	RelayURL string `json:"relayURL"`
}

// GetRelayURL is the relay to send through, or empty when push notifications are off.
func (push Push) GetRelayURL() string {
	if push.RelayURL == "off" {
		return ""
	}
	return strings.TrimRight(push.RelayURL, "/")
}

// MinimumVersions holds the lowest app version the server accepts per client; empty means no minimum.
type MinimumVersions struct {
	DesktopMacOS string `json:"desktopMacOS"`
	IOS          string `json:"iOS"`
	Android      string `json:"android"`
	CLI          string `json:"cli"`
	Extension    string `json:"extension"`
}

// Get is the minimum version of a client from apitypes.GetClient.
func (minimumVersions MinimumVersions) Get(client string) string {
	switch client {
	case apitypes.ClientDesktopMacOS:
		return minimumVersions.DesktopMacOS
	case apitypes.ClientIOS:
		return minimumVersions.IOS
	case apitypes.ClientAndroid:
		return minimumVersions.Android
	case apitypes.ClientCLI:
		return minimumVersions.CLI
	case apitypes.ClientExtension:
		return minimumVersions.Extension
	}
	return ""
}

// InvitePolicy says who may invite people to the server from the apps.
type InvitePolicy string

const (
	InvitePolicyAnyone InvitePolicy = "anyone"
	InvitePolicyAdmins InvitePolicy = "admins"
)

// AllowsInvite reports whether an account with this role may create sign-up invites.
func (policy InvitePolicy) AllowsInvite(role string) bool {
	return policy == InvitePolicyAnyone || role == apitypes.RoleAdmin
}

// SetupState is a step of setting up a fresh server from the dashboard; SetupDone is final.
type SetupState string

const (
	SetupNeedsAdmin        SetupState = "needsAdmin"
	SetupNeedsChecks       SetupState = "needsChecks"
	SetupNeedsInvitePolicy SetupState = "needsInvitePolicy"
	SetupDone              SetupState = "done"
)

// Retention is how long the server keeps item history, the audit log and expired invitations.
type Retention struct {
	History        Days `json:"historyDays"`
	Audit          Days `json:"auditDays"`
	ExpiredInvites Days `json:"expiredInvitesDays"`
}

// Days is a duration in whole days, as the dashboard edits it.
type Days int

func (days Days) Duration() time.Duration { return time.Duration(days) * 24 * time.Hour }

const (
	maximumSessionLifetime = 365
	minimumAuditRetention  = 30
	maximumRetention       = 3650
)

func Defaults() Settings {
	return Settings{
		SMTP:            SMTP{Port: 587},
		InvitePolicy:    InvitePolicyAnyone,
		SessionLifetime: 30,
		AuthBurst:       10,
		Retention:       Retention{History: 90, Audit: 365, ExpiredInvites: 30},
		Push:            Push{RelayURL: DefaultPushRelayURL},
		SetupState:      SetupDone,
	}
}

// Validate holds every bound of the settings, so the dashboard and the environment are checked alike.
func (settings Settings) Validate() error {
	if smtp := settings.SMTP; smtp.IsSet() {
		if smtp.Port < 1 || smtp.Port > 65535 {
			return errors.New("The SMTP port must be between 1 and 65535.")
		}
		if _, err := netmail.ParseAddress(smtp.From); err != nil {
			return errors.New("The sender must be an email address, like Hisar <noreply@example.com>.")
		}
		if smtp.Username != "" && smtp.Password == "" {
			return errors.New("Enter the SMTP password for the username. It must be entered again whenever the host, port or username changes.")
		}
	}
	for _, minimum := range []struct{ app, version string }{
		{"the Mac app", settings.MinimumVersions.DesktopMacOS}, {"the iPhone app", settings.MinimumVersions.IOS},
		{"the Android app", settings.MinimumVersions.Android},
		{"the command-line tool", settings.MinimumVersions.CLI}, {"the browser extension", settings.MinimumVersions.Extension},
	} {
		if minimum.version == "" {
			continue
		}
		if _, err := appversion.Parse(minimum.version); err != nil {
			return fmt.Errorf("The minimum version of %s must look like 1.2.3: %w", minimum.app, err)
		}
	}
	if settings.InvitePolicy != InvitePolicyAnyone && settings.InvitePolicy != InvitePolicyAdmins {
		return errors.New("Choose who can invite people: anyone or admins only.")
	}
	if settings.SessionLifetime < 1 || settings.SessionLifetime > maximumSessionLifetime {
		return fmt.Errorf("The session lifetime must be between 1 and %d days.", maximumSessionLifetime)
	}
	if settings.AuthBurst < 1 {
		return errors.New("The auth burst must be at least 1.")
	}
	for _, retention := range []struct {
		name    string
		days    Days
		minimum Days
	}{
		{"Item history", settings.Retention.History, 1}, {"The audit log", settings.Retention.Audit, minimumAuditRetention},
		{"Expired invitations", settings.Retention.ExpiredInvites, 1},
	} {
		if retention.days < retention.minimum || retention.days > maximumRetention {
			return fmt.Errorf("%s must be kept between %d and %d days.", retention.name, retention.minimum, maximumRetention)
		}
	}
	if relayURL := settings.Push.GetRelayURL(); relayURL != "" && !isPushRelayURL(relayURL) {
		return errors.New("The push relay must be an https:// address, or empty to turn push notifications off.")
	}
	switch settings.SetupState {
	case SetupNeedsAdmin, SetupNeedsChecks, SetupNeedsInvitePolicy, SetupDone:
	default:
		return fmt.Errorf("settings: unknown setup state %q", settings.SetupState)
	}
	return nil
}

// isPushRelayURL accepts https addresses, and http only on this computer, for a relay run in tests.
func isPushRelayURL(text string) bool {
	parsed, err := url.Parse(text)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	host := parsed.Hostname()
	return parsed.Scheme == "http" && (host == "127.0.0.1" || host == "localhost" || host == "::1")
}

// Field is one setting the dashboard edits, named by its path in the JSON document.
type Field struct {
	Name string
	// Environment is the variable that overrides the field, or empty.
	Environment string
	// IsSecret fields are never read back, only whether they are set.
	IsSecret bool
}

// IsEmail reports whether the field belongs to Email, which comes from the environment as a whole once any HISAR_SMTP_*
// variable is set, so a saved password is never sent to a host the environment did not choose.
func (field Field) IsEmail() bool { return strings.HasPrefix(field.Name, "smtp.") }

// ListFields returns every editable setting in the order the dashboard shows them.
func ListFields() []Field {
	return []Field{
		{Name: "smtp.host", Environment: "HISAR_SMTP_HOST"},
		{Name: "smtp.port", Environment: "HISAR_SMTP_PORT"},
		{Name: "smtp.username", Environment: "HISAR_SMTP_USERNAME"},
		{Name: "smtp.password", Environment: "HISAR_SMTP_PASSWORD", IsSecret: true},
		{Name: "smtp.from", Environment: "HISAR_SMTP_FROM"},
		{Name: "invitePolicy"},
		{Name: "minimumVersions.desktopMacOS", Environment: "HISAR_MINIMUM_VERSION_DESKTOP_MACOS"},
		{Name: "minimumVersions.iOS", Environment: "HISAR_MINIMUM_VERSION_IOS"},
		{Name: "minimumVersions.android", Environment: "HISAR_MINIMUM_VERSION_ANDROID"},
		{Name: "minimumVersions.cli", Environment: "HISAR_MINIMUM_VERSION_CLI"},
		{Name: "minimumVersions.extension", Environment: "HISAR_MINIMUM_VERSION_EXTENSION"},
		{Name: "sessionLifetimeDays"},
		{Name: "authBurst", Environment: "HISAR_AUTH_BURST"},
		{Name: "retention.historyDays"},
		{Name: "retention.auditDays"},
		{Name: "retention.expiredInvitesDays"},
		{Name: "push.relayURL", Environment: "HISAR_PUSH_RELAY_URL"},
	}
}

func findField(name string) (Field, bool) {
	for _, field := range ListFields() {
		if field.Name == name {
			return field, true
		}
	}
	return Field{}, false
}

// Source is where a field's effective value comes from.
type Source string

const (
	SourceDefault     Source = "default"
	SourceDatabase    Source = "database"
	SourceEnvironment Source = "environment"
)

// Sources maps each field name to its source.
type Sources map[string]Source

// Environment reads a variable, as os.Getenv does; an empty value leaves the field to the document or the default.
type Environment func(name string) string

// Resolve applies the stored document over the defaults and the environment over both, and says where each field
// came from. Any HISAR_SMTP_* variable makes all of Email come from the environment, with the defaults for the variables
// left empty. It does not validate the result.
func Resolve(defaults Settings, stored []byte, environment Environment) (Settings, Sources, error) {
	effective := defaults
	storedTree, err := decodeDocument(stored)
	if err != nil {
		return Settings{}, nil, err
	}
	if len(stored) > 0 {
		if err := json.Unmarshal(stored, &effective); err != nil {
			return Settings{}, nil, fmt.Errorf("settings: stored document: %w", err)
		}
	}
	isEmailFromEnvironment := false
	for _, field := range ListFields() {
		if field.IsEmail() && readEnvironmentText(field, environment) != "" {
			isEmailFromEnvironment = true
		}
	}
	if isEmailFromEnvironment {
		effective.SMTP = defaults.SMTP
	}
	defaultTree, err := encodeTree(defaults)
	if err != nil {
		return Settings{}, nil, err
	}
	sources := Sources{}
	environmentTree := map[string]any{}
	for _, field := range ListFields() {
		sources[field.Name] = SourceDefault
		if _, isStored := getPath(storedTree, field.Name); isStored {
			sources[field.Name] = SourceDatabase
		}
		if field.IsEmail() && isEmailFromEnvironment {
			sources[field.Name] = SourceEnvironment
		}
		text := readEnvironmentText(field, environment)
		if text == "" {
			continue
		}
		defaultValue, _ := getPath(defaultTree, field.Name)
		value, err := parseEnvironmentValue(field, defaultValue, text)
		if err != nil {
			return Settings{}, nil, err
		}
		setPath(environmentTree, field.Name, value)
		sources[field.Name] = SourceEnvironment
	}
	if err := applyTree(environmentTree, &effective); err != nil {
		return Settings{}, nil, err
	}
	return effective, sources, nil
}

// readEnvironmentText is the value of the field's variable, trimmed unless the field is a secret; empty if it has none.
func readEnvironmentText(field Field, environment Environment) string {
	if field.Environment == "" || environment == nil {
		return ""
	}
	text := environment(field.Environment)
	if field.IsSecret {
		return text
	}
	return strings.TrimSpace(text)
}

func parseEnvironmentValue(field Field, defaultValue any, text string) (any, error) {
	if _, isNumber := defaultValue.(json.Number); !isNumber {
		return text, nil
	}
	number, err := strconv.Atoi(text)
	if err != nil {
		return nil, fmt.Errorf("%s must be a whole number", field.Environment)
	}
	return number, nil
}

// Apply sets the changed fields in a stored document and returns the new document. A change names a field from
// ListFields and holds a value of the field's JSON type. A change of the SMTP host, port or username clears the saved
// SMTP password unless the same change sets it, so the password is never sent to a server it was not entered for.
func Apply(stored []byte, changes map[string]json.RawMessage) ([]byte, error) {
	storedTree, err := decodeDocument(stored)
	if err != nil {
		return nil, err
	}
	defaultTree, err := encodeTree(Defaults())
	if err != nil {
		return nil, err
	}
	serverBefore := getSMTPServer(storedTree, defaultTree)
	for name, raw := range changes {
		field, isKnown := findField(name)
		if !isKnown {
			return nil, fmt.Errorf("There is no setting named %q.", name)
		}
		var value any
		if err := decodeTree(raw, &value); err != nil {
			return nil, fmt.Errorf("The value of %s is not valid JSON.", name)
		}
		defaultValue, _ := getPath(defaultTree, name)
		if err := checkValueType(name, defaultValue, value); err != nil {
			return nil, err
		}
		if text, isText := value.(string); isText && !field.IsSecret {
			value = strings.TrimSpace(text)
		}
		setPath(storedTree, name, value)
	}
	_, isPasswordChanged := changes["smtp.password"]
	if password, _ := getPath(storedTree, "smtp.password"); !isPasswordChanged && password != nil && password != "" &&
		!reflect.DeepEqual(serverBefore, getSMTPServer(storedTree, defaultTree)) {
		setPath(storedTree, "smtp.password", "")
	}
	return json.Marshal(storedTree)
}

// getSMTPServer is the host, port and username a stored document sends email with, the defaults filling in the gaps.
func getSMTPServer(tree, defaultTree map[string]any) []any {
	var server []any
	for _, name := range []string{"smtp.host", "smtp.port", "smtp.username"} {
		value, isStored := getPath(tree, name)
		if !isStored {
			value, _ = getPath(defaultTree, name)
		}
		server = append(server, value)
	}
	return server
}

// SetStored sets one top-level key of a stored document, for a value the server records itself rather than one from
// ListFields, and returns the new document.
func SetStored(stored []byte, name string, value any) ([]byte, error) {
	storedTree, err := decodeDocument(stored)
	if err != nil {
		return nil, err
	}
	storedTree[name] = value
	return json.Marshal(storedTree)
}

func checkValueType(name string, defaultValue, value any) error {
	switch defaultValue.(type) {
	case json.Number:
		number, isNumber := value.(json.Number)
		if _, err := number.Int64(); !isNumber || err != nil {
			return fmt.Errorf("%s must be a whole number.", name)
		}
	case string:
		if _, isString := value.(string); !isString {
			return fmt.Errorf("%s must be text.", name)
		}
	}
	return nil
}

// ListChangedFields names the fields whose stored value differs between two documents.
func ListChangedFields(before, after []byte) ([]string, error) {
	beforeTree, err := decodeDocument(before)
	if err != nil {
		return nil, err
	}
	afterTree, err := decodeDocument(after)
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, field := range ListFields() {
		beforeValue, wasStored := getPath(beforeTree, field.Name)
		afterValue, isStored := getPath(afterTree, field.Name)
		if wasStored != isStored || !reflect.DeepEqual(beforeValue, afterValue) {
			changed = append(changed, field.Name)
		}
	}
	return changed, nil
}

// ListValues returns the value of every field from ListFields as JSON, by name.
func (settings Settings) ListValues() (map[string]json.RawMessage, error) {
	tree, err := encodeTree(settings)
	if err != nil {
		return nil, err
	}
	values := map[string]json.RawMessage{}
	for _, field := range ListFields() {
		value, _ := getPath(tree, field.Name)
		if values[field.Name], err = json.Marshal(value); err != nil {
			return nil, err
		}
	}
	return values, nil
}

// decodeDocument reads a stored document as a tree of objects; an empty document is an empty object.
func decodeDocument(document []byte) (map[string]any, error) {
	tree := map[string]any{}
	if len(document) == 0 {
		return tree, nil
	}
	if err := decodeTree(document, &tree); err != nil {
		return nil, fmt.Errorf("settings: stored document: %w", err)
	}
	return tree, nil
}

func decodeTree(document []byte, tree any) error {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	return decoder.Decode(tree)
}

func encodeTree(settings Settings) (map[string]any, error) {
	document, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	tree := map[string]any{}
	return tree, decodeTree(document, &tree)
}

func applyTree(tree map[string]any, settings *Settings) error {
	if len(tree) == 0 {
		return nil
	}
	document, err := json.Marshal(tree)
	if err != nil {
		return err
	}
	return json.Unmarshal(document, settings)
}

func getPath(tree map[string]any, name string) (any, bool) {
	parent, key, isFound := findParent(tree, name, false)
	if !isFound {
		return nil, false
	}
	value, isFound := parent[key]
	return value, isFound
}

func setPath(tree map[string]any, name string, value any) {
	parent, key, _ := findParent(tree, name, true)
	parent[key] = value
}

// findParent walks to the object that holds the last segment of a dotted name, creating objects on the way if asked.
func findParent(tree map[string]any, name string, isCreating bool) (map[string]any, string, bool) {
	segments := strings.Split(name, ".")
	parent := tree
	for _, segment := range segments[:len(segments)-1] {
		child, isObject := parent[segment].(map[string]any)
		if !isObject {
			if !isCreating {
				return nil, "", false
			}
			child = map[string]any{}
			parent[segment] = child
		}
		parent = child
	}
	return parent, segments[len(segments)-1], true
}

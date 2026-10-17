package cliprotocol

import "strings"

// A launcher such as Raycast sends launcher.hello after hello, and from then on only the types below. Launcher requests
// are read-only: none edits, deletes, shares, exports, imports or resolves secrets for a command. Only
// launcher.items.password and launcher.items.code return a value, one at a time; launcher.items.copy copies in the app.
const (
	TypeLauncherHello            = "launcher.hello"
	TypeLauncherListItem         = "launcher.items.list"
	TypeLauncherGetCode          = "launcher.items.code"
	TypeLauncherReadPassword     = "launcher.items.password"
	TypeLauncherCopyField        = "launcher.items.copy"
	TypeLauncherGeneratePassword = "launcher.passwords.generate"
	TypeLauncherLock             = "launcher.lock"
)

// The fields launcher.items.copy may name; they are the app's CopyField names.
const (
	LauncherFieldUsername    = "username"
	LauncherFieldPassword    = "password"
	LauncherFieldOneTimeCode = "one-time code"
)

// ListLauncherRequestTypes returns every request a launcher connection may send, launcher.hello first. It is the
// launcher allowlist, and hello announces each type as a feature.
func ListLauncherRequestTypes() []string {
	return []string{
		TypeLauncherHello, TypeLauncherListItem, TypeLauncherGetCode, TypeLauncherReadPassword,
		TypeLauncherCopyField, TypeLauncherGeneratePassword, TypeLauncherLock,
	}
}

// IsLauncherRequest tells a launcher's requests apart: every "launcher." type, known or not, belongs to launcher
// connections only.
func IsLauncherRequest(requestType string) bool {
	return strings.HasPrefix(requestType, "launcher.")
}

// IsLauncherField reports whether launcher.items.copy may copy field.
func IsLauncherField(field string) bool {
	return field == LauncherFieldUsername || field == LauncherFieldPassword || field == LauncherFieldOneTimeCode
}

// GeneratorChange shapes the password of launcher.passwords.generate; the app checks the length.
type GeneratorChange struct {
	Length  int  `json:"length"`
	Digits  bool `json:"digits"`
	Symbols bool `json:"symbols"`
}

// LauncherHelloResult tells the launcher whether its app is approved already, so it knows a dialog may open.
type LauncherHelloResult struct {
	IsApproved bool `json:"isApproved"`
}

// LauncherVault is a vault the launcher can filter by.
type LauncherVault struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// LauncherReference is a field an item has, by its hisar:// reference; never its value.
type LauncherReference struct {
	Field     string `json:"field"`
	Reference string `json:"reference"`
}

// LauncherItem is what a launcher may know of an item: metadata only. It has no field that could hold a secret;
// HasPassword and HasOneTimeCode say which secret actions the launcher may offer.
type LauncherItem struct {
	ID             string              `json:"id"`
	VaultID        string              `json:"vaultID"`
	Title          string              `json:"title"`
	Category       string              `json:"category"`
	Username       string              `json:"username"`
	URLs           []string            `json:"urls"`
	Tags           []string            `json:"tags"`
	IsFavorite     bool                `json:"isFavorite"`
	HasPassword    bool                `json:"hasPassword"`
	HasOneTimeCode bool                `json:"hasOneTimeCode"`
	References     []LauncherReference `json:"references"`
}

// LauncherItemListResult is every item in every vault the user can open, and those vaults sorted by name.
type LauncherItemListResult struct {
	Vaults []LauncherVault `json:"vaults"`
	Items  []LauncherItem  `json:"items"`
}

// LauncherPasswordResult is one item's password, read for one paste.
type LauncherPasswordResult struct {
	Password string `json:"password"`
}

// LauncherCopyResult: the app copied the field and clears the clipboard after ClearAfterSeconds if it still holds it.
type LauncherCopyResult struct {
	ClearAfterSeconds int `json:"clearAfterSeconds"`
}

type GeneratedPasswordResult struct {
	Password string `json:"password"`
}

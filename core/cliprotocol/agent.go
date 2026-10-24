package cliprotocol

type AgentVault struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ItemCount int    `json:"itemCount"`
}

type AgentVaultListResult struct {
	Vaults []AgentVault `json:"vaults"`
}

// AgentField is a field an agent may use by its reference; the value never leaves the app this way.
type AgentField struct {
	Label     string `json:"label"`
	Reference string `json:"reference"`
}

// AgentItem is the metadata of an item for agents: no value and no notes.
type AgentItem struct {
	ID                 string       `json:"id"`
	VaultID            string       `json:"vaultId"`
	VaultName          string       `json:"vaultName"`
	Title              string       `json:"title"`
	Type               string       `json:"type"`
	Username           string       `json:"username"`
	URLs               []string     `json:"urls"`
	Tags               []string     `json:"tags,omitempty"`
	HasNotes           bool         `json:"hasNotes"`
	HasOneTimePassword bool         `json:"hasOneTimeCode"`
	Fields             []AgentField `json:"fields"`
}

type AgentItemSearchResult struct {
	Items      []AgentItem `json:"items"`
	TotalCount int         `json:"totalCount"`
}

// Window is a browser window as cua-driver's list_windows names it: the process ID and the macOS window ID.
type Window struct {
	PID      int    `json:"pid"`
	WindowID uint32 `json:"windowId"`
}

const (
	SignInSubmitted     = "submitted"
	SignInNotAccepted   = "not-accepted"
	SignInStopped       = "stopped"
	SignInCleanupFailed = "cleanup-failed"
)

// The evidence that nothing Hisar entered stayed on the page; SignInCleanupUnverified comes only with
// SignInCleanupFailed. SignInCleanupFieldsRemoved: the page removed the fields from its document, and accessibility
// cannot show that it erased their values.
const (
	SignInCleanupNothingEntered = "nothing-entered"
	SignInCleanupPageLeft       = "page-left"
	SignInCleanupFieldsCleared  = "fields-cleared"
	SignInCleanupFieldsRemoved  = "fields-removed"
	SignInCleanupUnverified     = "unverified"
)

// AgentSignInResult is everything an agent learns from a sign-in: what Hisar did, never a value.
type AgentSignInResult struct {
	Outcome string            `json:"outcome"`
	Cleanup string            `json:"cleanup"`
	Website string            `json:"website"`
	Browser string            `json:"browser"`
	Login   AgentSignInLogin  `json:"login"`
	Steps   []AgentSignInStep `json:"steps"`
	Message string            `json:"message"`
}

// AgentSignInLogin is the metadata search_items already shows of the login Hisar used.
type AgentSignInLogin struct {
	ID        string `json:"id"`
	VaultID   string `json:"vaultId"`
	VaultName string `json:"vaultName"`
	Title     string `json:"title"`
	Username  string `json:"username"`
}

// AgentSignInStep is one page of the sign-in: the parts Hisar entered ("username", "password", "one-time code").
type AgentSignInStep struct {
	Filled      []string `json:"filled"`
	IsSubmitted bool     `json:"submitted"`
}

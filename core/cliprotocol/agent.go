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
	HasNotes           bool         `json:"hasNotes"`
	HasOneTimePassword bool         `json:"hasOneTimeCode"`
	Fields             []AgentField `json:"fields"`
}

type AgentItemSearchResult struct {
	Items      []AgentItem `json:"items"`
	TotalCount int         `json:"totalCount"`
}

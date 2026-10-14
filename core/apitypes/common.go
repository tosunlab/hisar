package apitypes

const (
	RoleAdmin  = "admin"
	RoleMember = "member"

	VaultOwner  = "owner"
	VaultEditor = "editor"
	VaultReader = "reader"

	VaultPersonal = "personal"
	VaultShared   = "shared"
)

type Error struct {
	Error string `json:"error"`
}

type DeviceInfo struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

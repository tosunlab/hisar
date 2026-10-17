package cliprotocol

import (
	"strings"

	"github.com/tryhisar/hisar/core/item"
)

const (
	ProtocolVersion   = 1
	MaxMessageSize    = 16 << 20
	MaxImportBatch    = 10
	MaxReferenceCount = 500
	// MaxAgentReferenceCount limits run_with_secrets, whose approval dialog lists every secret.
	MaxAgentReferenceCount = 100
	SocketEnvironment      = "HISAR_CLI_SOCKET"
)

// Features name what an app understands beyond ProtocolVersion, so a newer hisar can tell an older app apart without
// a version bump, which would turn off app mode for every command.
const (
	// FeatureSecureNotes: items.save creates the category ItemChange.Category names.
	FeatureSecureNotes = "secure-notes"
	// FeatureCreditCards: items.save creates credit-card items from their named fields.
	FeatureCreditCards = "credit-cards"
	// FeatureIdentities: items.save creates identity items from their named fields.
	FeatureIdentities = "identities"
	// FeatureAPICredentials: items.save creates api-credential items from their named fields.
	FeatureAPICredentials = "api-credentials"
	// FeatureAddresses: items.save creates address items from their named fields.
	FeatureAddresses = "addresses"
	// FeatureSoftwareLicenses: items.save creates software license items from their named fields.
	FeatureSoftwareLicenses = "software-licenses"
	// FeatureServers: items.save creates server items from their named fields and login data.
	FeatureServers = "servers"
	// FeatureDatabases: items.save creates database items from their named fields and login data.
	FeatureDatabases = "databases"
	// FeatureFavorites: favorites.list, favorites.add and favorites.remove; items.import stars Item.IsFavorite items.
	FeatureFavorites = "favorites"
	// FeatureTags: items.list and items.get return each item's tags, and items.save takes ItemChange.Tags.
	FeatureTags = "tags"
	// FeatureOrganizations: the organizations.*, teams.* requests, vaults.give-to-team, vaults.take-from-team and
	// vaults.create-organization-vault, and Vault.OrganizationID.
	FeatureOrganizations = "organizations"
)

// ListFeatures returns the features this build of the app announces in hello.
func ListFeatures() []string {
	return []string{FeatureSecureNotes, FeatureCreditCards, FeatureIdentities, FeatureFavorites, FeatureAPICredentials, FeatureTags, FeatureAddresses, FeatureSoftwareLicenses, FeatureServers, FeatureDatabases, FeatureOrganizations}
}

// GetCategoryFeature returns the feature an app must announce before items.save may create an item of category, or ""
// when every app can (logins).
func GetCategoryFeature(category string) string {
	return map[string]string{
		item.CategorySecureNote: FeatureSecureNotes, item.CategoryCreditCard: FeatureCreditCards, item.CategoryIdentity: FeatureIdentities,
		item.CategoryAPICredential:   FeatureAPICredentials,
		item.CategoryAddress:         FeatureAddresses,
		item.CategorySoftwareLicense: FeatureSoftwareLicenses,
		item.CategoryServer:          FeatureServers,
		item.CategoryDatabase:        FeatureDatabases,
	}[category]
}

const (
	StateSignedOut = "signed-out"
	StateLocked    = "locked"
	StateUnlocked  = "unlocked"
)

const (
	TypeHello         = "hello"
	TypeAccount       = "account"
	TypeListVault     = "vaults.list"
	TypeCreateVault   = "vaults.create"
	TypeListItem      = "items.list"
	TypeGetItem       = "items.get"
	TypeGetCode       = "items.code"
	TypeSaveItem      = "items.save"
	TypeDeleteItem    = "items.delete"
	TypeImportItem    = "items.import"
	TypeListPeople    = "people.list"
	TypeShareVault    = "vaults.share"
	TypeUnshareVault  = "vaults.unshare"
	TypeInvite        = "invite"
	TypeListDevice    = "devices.list"
	TypeRevokeDevice  = "devices.revoke"
	TypeResolveSecret = "secrets.resolve"

	TypeCreateShareLink = "shareLinks.create"
	TypeListShareLink   = "shareLinks.list"
	TypeRevokeShareLink = "shareLinks.revoke"

	TypeAgentHello         = "agent.hello"
	TypeAgentListVault     = "agent.vaults.list"
	TypeAgentSearchItem    = "agent.items.search"
	TypeAgentResolveSecret = "agent.secrets.resolve"
	TypeAgentGetCode       = "agent.items.code"
	TypeAgentRevealSecret  = "agent.secrets.reveal"

	TypeGetVaultActions = "vaults.actions"
	TypeRenameVault     = "vaults.rename"
	TypeDeleteVault     = "vaults.delete"
	TypeLeaveVault      = "vaults.leave"
	TypeExport          = "export"

	TypeListInvitation    = "invitations.list"
	TypeAcceptInvitation  = "invitations.accept"
	TypeDeclineInvitation = "invitations.decline"
	TypeCancelInvitation  = "invitations.cancel"

	TypeListFavorite   = "favorites.list"
	TypeAddFavorite    = "favorites.add"
	TypeRemoveFavorite = "favorites.remove"

	// The organization requests need FeatureOrganizations.
	TypeListOrganization              = "organizations.list"
	TypeGetOrganization               = "organizations.get"
	TypeCreateOrganization            = "organizations.create"
	TypeRenameOrganization            = "organizations.rename"
	TypeDeleteOrganization            = "organizations.delete"
	TypeInviteToOrganization          = "organizations.invite"
	TypeCancelOrganizationInvitation  = "organizations.cancel-invitation"
	TypeAcceptOrganizationInvitation  = "organizations.accept"
	TypeDeclineOrganizationInvitation = "organizations.decline"
	TypeSetOrganizationRole           = "organizations.set-role"
	TypeRemoveOrganizationMember      = "organizations.remove-member"
	TypeLeaveOrganization             = "organizations.leave"
	TypeCompleteOrganizationKeys      = "organizations.complete-keys"
	TypeCreateTeam                    = "teams.create"
	TypeRenameTeam                    = "teams.rename"
	TypeDeleteTeam                    = "teams.delete"
	TypeAddTeamMember                 = "teams.add-member"
	TypeRemoveTeamMember              = "teams.remove-member"
	TypeGiveVaultToTeam               = "vaults.give-to-team"
	TypeTakeVaultFromTeam             = "vaults.take-from-team"
	TypeCreateOrganizationVault       = "vaults.create-organization-vault"
)

const (
	ErrorInvalidRequest      = "invalid-request"
	ErrorIntegrationDisabled = "integration-disabled"
	ErrorSignedOut           = "signed-out"
	ErrorLocked              = "locked"
	ErrorDenied              = "denied"
	ErrorOffline             = "offline"
	ErrorNotFound            = "not-found"
	ErrorInternal            = "internal"
	ErrorUpdateRequired      = "update-required"
)

// IsAgentRequest tells the requests of hisar mcp apart; agent.hello starts an agent connection.
func IsAgentRequest(requestType string) bool {
	return strings.HasPrefix(requestType, "agent.")
}

package cliprotocol

import (
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/session"
	"slices"
)

type HelloResult struct {
	ProtocolVersion int    `json:"protocolVersion"`
	State           string `json:"state"`
	Email           string `json:"email"`
	Server          string `json:"server"`
	IsApproved      bool   `json:"isApproved"`
	// Features lists what the app understands beyond ProtocolVersion; an older app sends none.
	Features []string `json:"features,omitempty"`
}

// HasFeature reports whether the app announced feature.
func (hello HelloResult) HasFeature(feature string) bool {
	return slices.Contains(hello.Features, feature)
}

type AccountResult struct {
	AccountID string `json:"accountID"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Server    string `json:"server"`
}

type Vault struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Role      string `json:"role"`
	ItemCount int    `json:"itemCount"`
	// OrganizationID is set for organization vaults, by apps that announce FeatureOrganizations.
	OrganizationID string `json:"organizationID,omitempty"`
}

type VaultListResult struct {
	Vaults []Vault             `json:"vaults"`
	Issues []session.ReadIssue `json:"issues,omitempty"`
}

type VaultResult struct {
	Vault Vault `json:"vault"`
}

type Field struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Concealed bool   `json:"concealed"`
}

type Passkey struct {
	RelyingPartyID string `json:"relyingPartyID"`
	UserName       string `json:"userName"`
}

// Item is what the CLI may see of an item; summaries leave the secret parts empty.
type Item struct {
	ID                 string    `json:"id"`
	Revision           string    `json:"revision"`
	Category           string    `json:"category"`
	Title              string    `json:"title"`
	Username           string    `json:"username"`
	URLs               []string  `json:"urls"`
	Tags               []string  `json:"tags,omitempty"`
	HasOneTimePassword bool      `json:"hasOneTimePassword"`
	Password           string    `json:"password,omitempty"`
	Notes              string    `json:"notes,omitempty"`
	Fields             []Field   `json:"fields,omitempty"`
	Passkeys           []Passkey `json:"passkeys,omitempty"`
	PublicKey          string    `json:"publicKey,omitempty"`
	Fingerprint        string    `json:"fingerprint,omitempty"`
}

type ItemListResult struct {
	Items  []Item              `json:"items"`
	Issues []session.ReadIssue `json:"issues,omitempty"`
}

type ItemResult struct {
	Item Item `json:"item"`
}

// ItemChange holds what the CLI edits; the app keeps every other part of an existing item, the fields when Fields is nil
// and the tags when Tags is nil.
type ItemChange struct {
	ID       string    `json:"id,omitempty"`
	Revision string    `json:"revision,omitempty"`
	Title    string    `json:"title"`
	Username string    `json:"username"`
	URLs     []string  `json:"urls"`
	Tags     *[]string `json:"tags,omitempty"`
	Password string    `json:"password"`
	Notes    string    `json:"notes"`
	Fields   *[]Field  `json:"fields,omitempty"`
	// Category is the type of a new item (login or secure-note); an existing item keeps its own. An app that does not
	// announce FeatureSecureNotes ignores it, so hisar checks the feature before sending a secure note.
	Category string `json:"category,omitempty"`
}

// VaultDeletion: MoveToVaultID moves the items; IsDeletingItems deletes them and needs ConfirmedName, the vault's name.
type VaultDeletion struct {
	MoveToVaultID   string `json:"moveToVaultID,omitempty"`
	IsDeletingItems bool   `json:"isDeletingItems,omitempty"`
	ConfirmedName   string `json:"confirmedName,omitempty"`
}

type VaultDeletionResult struct {
	MovedCount int `json:"movedCount"`
}

type VaultMember struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type VaultActionsResult struct {
	Vault               Vault         `json:"vault"`
	CanRename           bool          `json:"canRename"`
	CanDelete           bool          `json:"canDelete"`
	DeleteBlockedReason string        `json:"deleteBlockedReason,omitempty"`
	CanLeave            bool          `json:"canLeave"`
	LeaveBlockedReason  string        `json:"leaveBlockedReason,omitempty"`
	MembersLosingAccess []VaultMember `json:"membersLosingAccess"`
	TargetVaults        []Vault       `json:"targetVaults"`
}

const (
	ExportFormatEncrypted = "encrypted"
	ExportFormatPlain     = "plain"
)

// ExportChange: the app writes the file at Path itself, so private keys never cross the socket; it never replaces a file.
type ExportChange struct {
	Format           string `json:"format"`
	Passphrase       string `json:"passphrase,omitempty"`
	Path             string `json:"path"`
	AcceptProvenance bool   `json:"acceptProvenance,omitempty"`
}

type ExportResult struct {
	Path       string             `json:"path"`
	VaultCount int                `json:"vaultCount"`
	ItemCount  int                `json:"itemCount"`
	Provenance *export.Provenance `json:"provenance,omitempty"`
}

type CodeResult struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
	Period    int    `json:"period"`
}

type ImportFailure struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// Favorite is one of the user's starred items, in a vault they can open.
type Favorite struct {
	VaultID string `json:"vaultID"`
	ItemID  string `json:"itemID"`
}

type FavoriteListResult struct {
	Favorites []Favorite `json:"favorites"`
}

type ImportResult struct {
	SavedCount int             `json:"savedCount"`
	Failures   []ImportFailure `json:"failures"`
}

type Person struct {
	AccountID     string `json:"accountID"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	SecurityCode  string `json:"securityCode"`
	IsCurrentUser bool   `json:"isCurrentUser"`
	IsKeyChanged  bool   `json:"isKeyChanged"`
}

type PeopleListResult struct {
	People []Person `json:"people"`
}

type ReceivedInvitation struct {
	ID             string `json:"id"`
	VaultID        string `json:"vaultID"`
	Role           string `json:"role"`
	InvitedByName  string `json:"invitedByName"`
	InvitedByEmail string `json:"invitedByEmail"`
	ExpiresAt      string `json:"expiresAt"`
}

type SentInvitation struct {
	ID                   string `json:"id"`
	VaultID              string `json:"vaultID"`
	VaultName            string `json:"vaultName"`
	Email                string `json:"email"`
	Role                 string `json:"role"`
	State                string `json:"state"`
	ExpiresAt            string `json:"expiresAt"`
	RequiresConfirmation bool   `json:"requiresConfirmation,omitempty"`
	RecipientID          string `json:"recipientID,omitempty"`
	SecurityCode         string `json:"securityCode,omitempty"`
	ReviewToken          string `json:"reviewToken,omitempty"`
}

type InvitationListResult struct {
	Received        []ReceivedInvitation `json:"received"`
	Sent            []SentInvitation     `json:"sent"`
	CompletionError string               `json:"completionError,omitempty"`
}

type Device struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	LastSeenAt string `json:"lastSeenAt"`
	IsCurrent  bool   `json:"isCurrent"`
}

// ShareLinkChange describes a new link: Fields of the item named by Vault and Query, or Text with Title.
type ShareLinkChange struct {
	Fields           []string `json:"fields,omitempty"`
	ExpiresInSeconds int64    `json:"expiresInSeconds"`
	MaxViews         int      `json:"maxViews"`
	Passphrase       string   `json:"passphrase,omitempty"`
	Text             string   `json:"text,omitempty"`
	Title            string   `json:"title,omitempty"`
}

// ShareLink has a URL only in the result of shareLinks.create.
type ShareLink struct {
	ID            string   `json:"id"`
	URL           string   `json:"url,omitempty"`
	Title         string   `json:"title"`
	FieldLabels   []string `json:"fieldLabels"`
	IsText        bool     `json:"isText"`
	CreatedAt     string   `json:"createdAt"`
	ExpiresAt     string   `json:"expiresAt"`
	MaxViews      int      `json:"maxViews"`
	ViewCount     int      `json:"viewCount"`
	HasPassphrase bool     `json:"hasPassphrase"`
}

type ShareLinkResult struct {
	ShareLink ShareLink `json:"shareLink"`
}

type ShareLinkListResult struct {
	ShareLinks []ShareLink `json:"shareLinks"`
}

type ResolveSecretResult struct {
	Values []string `json:"values"`
}

type DeviceListResult struct {
	Devices []Device `json:"devices"`
}

type OrganizationSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	MemberCount int    `json:"memberCount"`
}

// OrganizationInvitation is an invitation to the user's email; ExpiresAt is RFC 3339.
type OrganizationInvitation struct {
	ID               string `json:"id"`
	OrganizationID   string `json:"organizationID"`
	OrganizationName string `json:"organizationName"`
	Role             string `json:"role"`
	InvitedByName    string `json:"invitedByName"`
	InvitedByEmail   string `json:"invitedByEmail"`
	ExpiresAt        string `json:"expiresAt"`
}

type OrganizationListResult struct {
	Organizations []OrganizationSummary    `json:"organizations"`
	Invitations   []OrganizationInvitation `json:"invitations"`
}

type OrganizationSummaryResult struct {
	Organization OrganizationSummary `json:"organization"`
}

type OrganizationMember struct {
	AccountID     string `json:"accountID"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	SecurityCode  string `json:"securityCode"`
	IsCurrentUser bool   `json:"isCurrentUser"`
	IsKeyChanged  bool   `json:"isKeyChanged"`
}

type TeamMember struct {
	AccountID string `json:"accountID"`
	Name      string `json:"name"`
	Email     string `json:"email"`
}

// TeamVault: Name is empty when CanOpen is false, because this user has no key to decrypt it.
type TeamVault struct {
	VaultID string `json:"vaultID"`
	Name    string `json:"name,omitempty"`
	Role    string `json:"role"`
	CanOpen bool   `json:"canOpen"`
}

type Team struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Members []TeamMember `json:"members"`
	Vaults  []TeamVault  `json:"vaults"`
}

// SentOrganizationInvitation is a pending invitation; only admins see them. ExpiresAt is RFC 3339.
type SentOrganizationInvitation struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt string `json:"expiresAt"`
}

type Organization struct {
	ID          string                       `json:"id"`
	Name        string                       `json:"name"`
	Role        string                       `json:"role"`
	Members     []OrganizationMember         `json:"members"`
	Teams       []Team                       `json:"teams"`
	Invitations []SentOrganizationInvitation `json:"invitations"`
}

type OrganizationResult struct {
	Organization Organization `json:"organization"`
}

type TeamResult struct {
	Team Team `json:"team"`
}

// WaitingVaultKey is an organization vault the user's teams give them that no owner's app has delivered yet.
type WaitingVaultKey struct {
	VaultID          string `json:"vaultID"`
	OrganizationID   string `json:"organizationID"`
	OrganizationName string `json:"organizationName"`
	Role             string `json:"role"`
}

// OrganizationKeysResult says how many vault keys the app delivered and how many vaults it rotated; the errors say why
// some could not be, e.g. a changed signing key.
type OrganizationKeysResult struct {
	DeliveredCount int               `json:"deliveredCount"`
	RotatedCount   int               `json:"rotatedCount"`
	DeliveryError  string            `json:"deliveryError,omitempty"`
	RotationError  string            `json:"rotationError,omitempty"`
	Waiting        []WaitingVaultKey `json:"waiting"`
}

package cliprotocol

type HelloResult struct {
	ProtocolVersion int    `json:"protocolVersion"`
	State           string `json:"state"`
	Email           string `json:"email"`
	Server          string `json:"server"`
	IsApproved      bool   `json:"isApproved"`
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
}

type VaultListResult struct {
	Vaults []Vault `json:"vaults"`
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
	HasOneTimePassword bool      `json:"hasOneTimePassword"`
	Password           string    `json:"password,omitempty"`
	Notes              string    `json:"notes,omitempty"`
	Fields             []Field   `json:"fields,omitempty"`
	Passkeys           []Passkey `json:"passkeys,omitempty"`
	PublicKey          string    `json:"publicKey,omitempty"`
	Fingerprint        string    `json:"fingerprint,omitempty"`
}

type ItemListResult struct {
	Items []Item `json:"items"`
}

type ItemResult struct {
	Item Item `json:"item"`
}

// ItemChange holds what the CLI edits; the app keeps every other part of an existing item, and the fields when Fields is nil.
type ItemChange struct {
	ID       string   `json:"id,omitempty"`
	Revision string   `json:"revision,omitempty"`
	Title    string   `json:"title"`
	Username string   `json:"username"`
	URLs     []string `json:"urls"`
	Password string   `json:"password"`
	Notes    string   `json:"notes"`
	Fields   *[]Field `json:"fields,omitempty"`
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
	Vault                 Vault         `json:"vault"`
	CanRename             bool          `json:"canRename"`
	CanDelete             bool          `json:"canDelete"`
	DeleteBlockedReason   string        `json:"deleteBlockedReason,omitempty"`
	CanLeave              bool          `json:"canLeave"`
	LeaveBlockedReason    string        `json:"leaveBlockedReason,omitempty"`
	AllMemberLosingAccess []VaultMember `json:"membersLosingAccess"`
	AllTargetVault        []Vault       `json:"targetVaults"`
}

const (
	ExportFormatEncrypted = "encrypted"
	ExportFormatPlain     = "plain"
)

// ExportChange: the app writes the file at Path itself, so private keys never cross the socket; it never replaces a file.
type ExportChange struct {
	Format     string `json:"format"`
	Passphrase string `json:"passphrase,omitempty"`
	Path       string `json:"path"`
}

type ExportResult struct {
	Path       string `json:"path"`
	VaultCount int    `json:"vaultCount"`
	ItemCount  int    `json:"itemCount"`
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
	ID        string `json:"id"`
	VaultID   string `json:"vaultID"`
	VaultName string `json:"vaultName"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	State     string `json:"state"`
	ExpiresAt string `json:"expiresAt"`
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

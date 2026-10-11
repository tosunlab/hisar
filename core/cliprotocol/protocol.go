// Package cliprotocol is the protocol between the hisar CLI and the desktop app on ~/.hisar/cli.sock.
package cliprotocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

	TypeListInvitation    = "invitations.list"
	TypeAcceptInvitation  = "invitations.accept"
	TypeDeclineInvitation = "invitations.decline"
	TypeCancelInvitation  = "invitations.cancel"
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

var ErrMessageTooLarge = errors.New("cliprotocol: message larger than 16 MiB")

type Request struct {
	ID            string           `json:"id"`
	Type          string           `json:"type"`
	VaultID       string           `json:"vaultID,omitempty"`
	ItemID        string           `json:"itemID,omitempty"`
	Name          string           `json:"name,omitempty"`
	Email         string           `json:"email,omitempty"`
	Role          string           `json:"role,omitempty"`
	AccountID     string           `json:"accountID,omitempty"`
	DeviceID      string           `json:"deviceID,omitempty"`
	InvitationID  string           `json:"invitationID,omitempty"`
	Item          *ItemChange      `json:"item,omitempty"`
	ImportedItems []item.Item      `json:"importedItems,omitempty"`
	References    []string         `json:"references,omitempty"`
	CommandLine   []string         `json:"commandLine,omitempty"`
	ShareLink     *ShareLinkChange `json:"shareLink,omitempty"`
	ShareLinkID   string           `json:"shareLinkID,omitempty"`

	AgentName        string `json:"agentName,omitempty"`
	AgentTitle       string `json:"agentTitle,omitempty"`
	AgentVersion     string `json:"agentVersion,omitempty"`
	Query            string `json:"query,omitempty"`
	Vault            string `json:"vault,omitempty"`
	WorkingDirectory string `json:"workingDirectory,omitempty"`

	ClientPlatform string `json:"clientPlatform,omitempty"`
	ClientVersion  string `json:"clientVersion,omitempty"`
}

type Response struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *ResponseError  `json:"error,omitempty"`
}

type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ResponseError) Error() string { return e.Message }

func CreateError(code, message string) *ResponseError {
	return &ResponseError{Code: code, Message: message}
}

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

// ItemChange holds what the CLI edits; the app keeps every other part of an existing item.
type ItemChange struct {
	ID       string   `json:"id,omitempty"`
	Revision string   `json:"revision,omitempty"`
	Title    string   `json:"title"`
	Username string   `json:"username"`
	URLs     []string `json:"urls"`
	Password string   `json:"password"`
	Notes    string   `json:"notes"`
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

// IsAgentRequest tells the requests of hisar mcp apart; agent.hello starts an agent connection.
func IsAgentRequest(requestType string) bool {
	return strings.HasPrefix(requestType, "agent.")
}

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

type DeviceListResult struct {
	Devices []Device `json:"devices"`
}

func GetDefaultSocketPath(homeDirectory string) string {
	return filepath.Join(homeDirectory, ".hisar", "cli.sock")
}

// GetSocketPath returns HISAR_CLI_SOCKET when set, otherwise ~/.hisar/cli.sock.
func GetSocketPath() (string, error) {
	if path := os.Getenv(SocketEnvironment); path != "" {
		return path, nil
	}
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return GetDefaultSocketPath(homeDirectory), nil
}

func ReadMessage(reader io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if length > MaxMessageSize {
		return nil, ErrMessageTooLarge
	}
	message := make([]byte, length)
	if _, err := io.ReadFull(reader, message); err != nil {
		return nil, err
	}
	return message, nil
}

func WriteMessage(writer io.Writer, message []byte) error {
	if len(message) > MaxMessageSize {
		return ErrMessageTooLarge
	}
	frame := make([]byte, 4+len(message))
	binary.BigEndian.PutUint32(frame, uint32(len(message)))
	copy(frame[4:], message)
	_, err := writer.Write(frame)
	return err
}

func WriteJSON(writer io.Writer, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return WriteMessage(writer, encoded)
}

func ReadJSON(reader io.Reader, value any) error {
	message, err := ReadMessage(reader)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(message, value); err != nil {
		return fmt.Errorf("cliprotocol: %w", err)
	}
	return nil
}

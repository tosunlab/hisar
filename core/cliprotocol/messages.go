package cliprotocol

import (
	"encoding/json"

	"github.com/tryhisar/hisar/core/item"
)

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
	VaultDeletion *VaultDeletion   `json:"vaultDeletion,omitempty"`
	Export        *ExportChange    `json:"export,omitempty"`

	AgentName        string `json:"agentName,omitempty"`
	AgentTitle       string `json:"agentTitle,omitempty"`
	AgentVersion     string `json:"agentVersion,omitempty"`
	Query            string `json:"query,omitempty"`
	Vault            string `json:"vault,omitempty"`
	WorkingDirectory string `json:"workingDirectory,omitempty"`

	ClientPlatform string `json:"clientPlatform,omitempty"`
	ClientVersion  string `json:"clientVersion,omitempty"`

	// Field is the field launcher.items.copy copies (IsLauncherField).
	Field string `json:"field,omitempty"`
	// Generator shapes launcher.passwords.generate; nil uses the app's defaults (24 characters, digits and symbols).
	Generator *GeneratorChange `json:"generator,omitempty"`
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

func NewError(code, message string) *ResponseError {
	return &ResponseError{Code: code, Message: message}
}

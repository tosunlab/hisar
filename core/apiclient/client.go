package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	// Platform and Version are sent with every request; an empty Version sends neither.
	Platform string
	Version  string
	// OnUpgradeRequired runs when the server answers 426 because Version is below its minimum.
	OnUpgradeRequired func(*UpgradeRequiredError)
}

func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

type APIError struct {
	Status  int
	Message string
	// IsKeyChanged marks the 409 that refused items sealed with a key a rotation replaced (apitypes.KeyChangedResponse).
	IsKeyChanged bool
}

func (e *APIError) Error() string { return fmt.Sprintf("server: %s (%d)", e.Message, e.Status) }

// ErrAccountSuspended is the server's 403 with apitypes.AccountSuspendedMessage: an admin suspended the account. It is
// not an APIError, so no check for a plain 403 mistakes it for another refusal, and the device keeps its session.
var ErrAccountSuspended = errors.New(apitypes.AccountSuspendedMessage)

// readError returns the error of an answer that is not a success, from its status and JSON body.
func readError(status int, data []byte) error {
	var errorBody apitypes.Error
	if json.Unmarshal(data, &errorBody) != nil || errorBody.Error == "" {
		errorBody.Error = http.StatusText(status)
	}
	if status == http.StatusForbidden && errorBody.Error == apitypes.AccountSuspendedMessage {
		return ErrAccountSuspended
	}
	return &APIError{Status: status, Message: errorBody.Error}
}

// UpgradeRequiredError is the server's 426: this app's version is below the minimum the server accepts.
type UpgradeRequiredError struct {
	MinimumVersion string
	UpdateURL      string
}

func (e *UpgradeRequiredError) Error() string {
	return fmt.Sprintf("update required: the server needs version %s or later", e.MinimumVersion)
}

func AsUpgradeRequired(err error) (*UpgradeRequiredError, bool) {
	var upgradeRequired *UpgradeRequiredError
	return upgradeRequired, errors.As(err, &upgradeRequired)
}

func (c *Client) setClientHeader(header http.Header) {
	if c.Version == "" {
		return
	}
	header.Set(apitypes.PlatformHeader, c.Platform)
	header.Set(apitypes.VersionHeader, c.Version)
}

func (c *Client) readUpgradeRequired(data []byte) error {
	var body apitypes.UpgradeRequired
	json.Unmarshal(data, &body)
	upgradeRequired := &UpgradeRequiredError{MinimumVersion: body.MinimumVersion, UpdateURL: body.UpdateURL}
	if c.OnUpgradeRequired != nil {
		c.OnUpgradeRequired(upgradeRequired)
	}
	return upgradeRequired
}

// IsStatus reports whether err is, or wraps, an API error with the given HTTP status.
func IsStatus(err error, status int) bool {
	var apiError *APIError
	return errors.As(err, &apiError) && apiError.Status == status
}

// IsKeyChanged reports whether err is, or wraps, the server refusing items sealed with a key a rotation replaced: the
// vault must be reloaded and the items sealed again with its current key.
func IsKeyChanged(err error) bool {
	var apiError *APIError
	return errors.As(err, &apiError) && apiError.IsKeyChanged
}

// IsUnreachable reports a failed connection or a reverse proxy that cannot reach the server.
func IsUnreachable(err error) bool {
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return true
	}
	return IsStatus(err, http.StatusBadGateway) || IsStatus(err, http.StatusServiceUnavailable) || IsStatus(err, http.StatusGatewayTimeout)
}

// sendRequest sends a JSON request; for a 409 that is not an apitypes.KeyChangedResponse the body is decoded into conflict
// when non-nil.
func (c *Client) sendRequest(ctx context.Context, method, path string, header http.Header, input, output, conflict any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	for headerName, headerValues := range header {
		request.Header[headerName] = headerValues
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}
	c.setClientHeader(request.Header)
	response, err := c.HTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return err
	}

	if response.StatusCode == http.StatusUpgradeRequired {
		return c.readUpgradeRequired(data)
	}
	if response.StatusCode == http.StatusConflict {
		var keyChanged apitypes.KeyChangedResponse
		if json.Unmarshal(data, &keyChanged) == nil && keyChanged.IsKeyChanged {
			return &APIError{Status: response.StatusCode, Message: keyChanged.Error, IsKeyChanged: true}
		}
	}
	if response.StatusCode == http.StatusConflict && conflict != nil {
		if err := json.Unmarshal(data, conflict); err != nil {
			return err
		}
		return &APIError{Status: response.StatusCode, Message: "conflict"}
	}
	if response.StatusCode >= 300 {
		return readError(response.StatusCode, data)
	}
	if output != nil && len(data) > 0 {
		return json.Unmarshal(data, output)
	}
	return nil
}

// CheckVersion fails with *UpgradeRequiredError when the server no longer accepts this app's version.
func (c *Client) CheckVersion(ctx context.Context) error {
	return c.sendRequest(ctx, "GET", "/api/v1/version-check", nil, nil, nil, nil)
}

func (c *Client) FetchPrelogin(ctx context.Context, email string) (*apitypes.PreloginResponse, error) {
	var response apitypes.PreloginResponse
	return &response, c.sendRequest(ctx, "GET", "/api/v1/auth/prelogin?email="+url.QueryEscape(email), nil, nil, &response, nil)
}

func (c *Client) StartRegistration(ctx context.Context, request *apitypes.RegisterStartRequest) (*apitypes.RegisterStartResponse, error) {
	var response apitypes.RegisterStartResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/auth/register/start", nil, request, &response, nil)
}

func (c *Client) FinishRegistration(ctx context.Context, request *apitypes.RegisterFinishRequest) (*apitypes.SessionResponse, error) {
	var response apitypes.SessionResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/auth/register/finish", nil, request, &response, nil)
}

func (c *Client) StartLogin(ctx context.Context, request *apitypes.LoginStartRequest) (*apitypes.LoginStartResponse, error) {
	var response apitypes.LoginStartResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/auth/login/start", nil, request, &response, nil)
}

func (c *Client) FinishLogin(ctx context.Context, request *apitypes.LoginFinishRequest) (*apitypes.SessionResponse, error) {
	var response apitypes.SessionResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/auth/login/finish", nil, request, &response, nil)
}

func (c *Client) Logout(ctx context.Context) error {
	return c.sendRequest(ctx, "POST", "/api/v1/auth/logout", nil, nil, nil, nil)
}

func (c *Client) GetAccount(ctx context.Context) (*apitypes.Account, error) {
	var account apitypes.Account
	return &account, c.sendRequest(ctx, "GET", "/api/v1/account", nil, nil, &account, nil)
}

func (c *Client) GetAccountPreferences(ctx context.Context) (*apitypes.AccountPreferences, error) {
	var preferences apitypes.AccountPreferences
	return &preferences, c.sendRequest(ctx, "GET", "/api/v1/account/preferences", nil, nil, &preferences, nil)
}

// PutAccountPreferences answers 409 when another device saved the record after request.BaseRevision.
func (c *Client) PutAccountPreferences(ctx context.Context, request *apitypes.PutAccountPreferencesRequest) (int64, error) {
	var response apitypes.PutAccountPreferencesResponse
	err := c.sendRequest(ctx, "PUT", "/api/v1/account/preferences", nil, request, &response, nil)
	return response.Revision, err
}

func (c *Client) ListMembers(ctx context.Context) ([]apitypes.Member, error) {
	var members []apitypes.Member
	return members, c.sendRequest(ctx, "GET", "/api/v1/members", nil, nil, &members, nil)
}

func (c *Client) Invite(ctx context.Context, request *apitypes.InviteRequest) (*apitypes.InvitationSentResponse, error) {
	var response apitypes.InvitationSentResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/invites", nil, request, &response, nil)
}

func (c *Client) CreateShareInvitation(ctx context.Context, vaultID string, request *apitypes.ShareInvitationRequest) (*apitypes.InvitationSentResponse, error) {
	var response apitypes.InvitationSentResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/share-invitations", nil, request, &response, nil)
}

func (c *Client) ListShareInvitations(ctx context.Context) (*apitypes.ShareInvitationList, error) {
	var response apitypes.ShareInvitationList
	return &response, c.sendRequest(ctx, "GET", "/api/v1/share-invitations", nil, nil, &response, nil)
}

func (c *Client) AcceptShareInvitation(ctx context.Context, invitationID string) error {
	return c.sendRequest(ctx, "POST", "/api/v1/share-invitations/"+url.PathEscape(invitationID)+"/accept", nil, nil, nil, nil)
}

func (c *Client) DeclineShareInvitation(ctx context.Context, invitationID string) error {
	return c.sendRequest(ctx, "POST", "/api/v1/share-invitations/"+url.PathEscape(invitationID)+"/decline", nil, nil, nil, nil)
}

func (c *Client) CancelShareInvitation(ctx context.Context, invitationID string) error {
	return c.sendRequest(ctx, "DELETE", "/api/v1/share-invitations/"+url.PathEscape(invitationID), nil, nil, nil, nil)
}

func (c *Client) ListDevices(ctx context.Context) ([]apitypes.Device, error) {
	var devices []apitypes.Device
	return devices, c.sendRequest(ctx, "GET", "/api/v1/devices", nil, nil, &devices, nil)
}

func (c *Client) RevokeDevice(ctx context.Context, deviceID string) error {
	return c.sendRequest(ctx, "DELETE", "/api/v1/devices/"+url.PathEscape(deviceID), nil, nil, nil, nil)
}

func (c *Client) ListVaults(ctx context.Context) ([]apitypes.Vault, error) {
	var vaults []apitypes.Vault
	return vaults, c.sendRequest(ctx, "GET", "/api/v1/vaults", nil, nil, &vaults, nil)
}

func (c *Client) CreateVault(ctx context.Context, request *apitypes.CreateVaultRequest) error {
	return c.sendRequest(ctx, "POST", "/api/v1/vaults", nil, request, nil, nil)
}

func (c *Client) ListVaultMembers(ctx context.Context, vaultID string) ([]apitypes.VaultMember, error) {
	var vaultMembers []apitypes.VaultMember
	return vaultMembers, c.sendRequest(ctx, "GET", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/members", nil, nil, &vaultMembers, nil)
}

func (c *Client) SetAccess(ctx context.Context, vaultID, accountID string, request *apitypes.SetAccessRequest) error {
	return c.sendRequest(ctx, "PUT", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/access/"+url.PathEscape(accountID), nil, request, nil, nil)
}

func (c *Client) RemoveAccess(ctx context.Context, vaultID, accountID string) error {
	return c.sendRequest(ctx, "DELETE", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/access/"+url.PathEscape(accountID), nil, nil, nil, nil)
}

func (c *Client) RotateVault(ctx context.Context, vaultID string, request *apitypes.RotateVaultRequest) error {
	return c.sendRequest(ctx, "POST", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/rotate", nil, request, nil, nil)
}

func (c *Client) RenameVault(ctx context.Context, vaultID string, request *apitypes.RenameVaultRequest) error {
	return c.sendRequest(ctx, "PUT", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/name", nil, request, nil, nil)
}

func (c *Client) DeleteVault(ctx context.Context, vaultID string, request *apitypes.DeleteVaultRequest) error {
	return c.sendRequest(ctx, "POST", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/delete", nil, request, nil, nil)
}

func (c *Client) ListChanges(ctx context.Context, vaultID string, since int64, limit int) (*apitypes.ChangesResponse, error) {
	var response apitypes.ChangesResponse
	query := url.Values{"since": {strconv.FormatInt(since, 10)}}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	return &response, c.sendRequest(ctx, "GET", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/changes?"+query.Encode(), nil, nil, &response, nil)
}

// PutItem returns the server's current item on a 409 (nil if the item does not exist, or for IsKeyChanged).
func (c *Client) PutItem(ctx context.Context, vaultID, itemID, baseRevision string, request *apitypes.PutItemRequest) (int64, *apitypes.Item, error) {
	header := http.Header{}
	if baseRevision != "" {
		header.Set("If-Match", `"`+baseRevision+`"`)
	}
	var response apitypes.PutItemResponse
	var conflict apitypes.ConflictResponse
	err := c.sendRequest(ctx, "PUT", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/items/"+url.PathEscape(itemID), header, request, &response, &conflict)
	if IsStatus(err, http.StatusConflict) {
		return 0, conflict.Current, err
	}
	return response.Seq, nil, err
}

// MoveItem moves an item to another vault, keeping its ID. On a 409 the current item is returned when the item changed in
// the source vault; it is nil when the item is gone, the target vault already has it, or for IsKeyChanged.
func (c *Client) MoveItem(ctx context.Context, vaultID, itemID string, request *apitypes.MoveItemRequest) (*apitypes.MoveItemResponse, *apitypes.Item, error) {
	var response apitypes.MoveItemResponse
	var conflict apitypes.ConflictResponse
	err := c.sendRequest(ctx, "POST", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/items/"+url.PathEscape(itemID)+"/move", nil, request, &response, &conflict)
	if IsStatus(err, http.StatusConflict) {
		return nil, conflict.Current, err
	}
	return &response, nil, err
}

func (c *Client) CreateShareLink(ctx context.Context, request *apitypes.CreateShareLinkRequest) (*apitypes.ShareLink, error) {
	var response apitypes.ShareLink
	return &response, c.sendRequest(ctx, "POST", "/api/v1/share-links", nil, request, &response, nil)
}

func (c *Client) ListShareLinks(ctx context.Context) ([]apitypes.ShareLink, error) {
	var shareLinks []apitypes.ShareLink
	return shareLinks, c.sendRequest(ctx, "GET", "/api/v1/share-links", nil, nil, &shareLinks, nil)
}

func (c *Client) RevokeShareLink(ctx context.Context, shareLinkID string) error {
	return c.sendRequest(ctx, "DELETE", "/api/v1/share-links/"+url.PathEscape(shareLinkID), nil, nil, nil, nil)
}

// OpenPublicShareLink needs no session and counts one view.
func (c *Client) OpenPublicShareLink(ctx context.Context, shareLinkID string) (*apitypes.PublicShareLink, error) {
	var response apitypes.PublicShareLink
	return &response, c.sendRequest(ctx, "GET", "/api/v1/public/share-links/"+url.PathEscape(shareLinkID), nil, nil, &response, nil)
}

func (c *Client) Reauthenticate(ctx context.Context, request *apitypes.ReauthenticateRequest) (*apitypes.ReauthenticateResponse, error) {
	var response apitypes.ReauthenticateResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/account/reauthenticate", nil, request, &response, nil)
}

func (c *Client) ChangeCredentials(ctx context.Context, request *apitypes.ChangeCredentialsRequest) (*apitypes.ChangeCredentialsResponse, error) {
	var response apitypes.ChangeCredentialsResponse
	return &response, c.sendRequest(ctx, "PUT", "/api/v1/account/credentials", nil, request, &response, nil)
}

func (c *Client) GetAccountDeletionPreview(ctx context.Context) (*apitypes.AccountDeletionPreview, error) {
	var response apitypes.AccountDeletionPreview
	return &response, c.sendRequest(ctx, "GET", "/api/v1/account/deletion-preview", nil, nil, &response, nil)
}

func (c *Client) DeleteAccount(ctx context.Context, request *apitypes.DeleteAccountRequest) error {
	return c.sendRequest(ctx, "POST", "/api/v1/account/delete", nil, request, nil, nil)
}

func (c *Client) GetItemHistory(ctx context.Context, vaultID, itemID string) ([]apitypes.Item, error) {
	var items []apitypes.Item
	return items, c.sendRequest(ctx, "GET", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/items/"+url.PathEscape(itemID)+"/history", nil, nil, &items, nil)
}

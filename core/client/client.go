package client

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

	"github.com/tryhisar/hisar/core/proto"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func Create(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("server: %s (%d)", e.Message, e.Status) }

func IsStatus(err error, status int) bool {
	apiError, ok := err.(*APIError)
	return ok && apiError.Status == status
}

// IsUnreachable reports a failed connection or a reverse proxy that cannot reach the server.
func IsUnreachable(err error) bool {
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return true
	}
	return IsStatus(err, http.StatusBadGateway) || IsStatus(err, http.StatusServiceUnavailable) || IsStatus(err, http.StatusGatewayTimeout)
}

// sendRequest sends a JSON request; for a 409 the body is decoded into conflict when non-nil.
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
	for headerName, allHeaderValue := range header {
		request.Header[headerName] = allHeaderValue
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return err
	}

	if response.StatusCode == http.StatusConflict && conflict != nil {
		if err := json.Unmarshal(data, conflict); err != nil {
			return err
		}
		return &APIError{Status: response.StatusCode, Message: "conflict"}
	}
	if response.StatusCode >= 300 {
		var errorBody proto.Error
		if json.Unmarshal(data, &errorBody) != nil || errorBody.Error == "" {
			errorBody.Error = http.StatusText(response.StatusCode)
		}
		return &APIError{Status: response.StatusCode, Message: errorBody.Error}
	}
	if output != nil && len(data) > 0 {
		return json.Unmarshal(data, output)
	}
	return nil
}

func (c *Client) FetchPrelogin(ctx context.Context, email string) (*proto.PreloginResponse, error) {
	var response proto.PreloginResponse
	return &response, c.sendRequest(ctx, "GET", "/api/v1/auth/prelogin?email="+url.QueryEscape(email), nil, nil, &response, nil)
}

func (c *Client) StartRegistration(ctx context.Context, request *proto.RegisterStartRequest) (*proto.RegisterStartResponse, error) {
	var response proto.RegisterStartResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/auth/register/start", nil, request, &response, nil)
}

func (c *Client) FinishRegistration(ctx context.Context, request *proto.RegisterFinishRequest) (*proto.SessionResponse, error) {
	var response proto.SessionResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/auth/register/finish", nil, request, &response, nil)
}

func (c *Client) StartLogin(ctx context.Context, request *proto.LoginStartRequest) (*proto.LoginStartResponse, error) {
	var response proto.LoginStartResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/auth/login/start", nil, request, &response, nil)
}

func (c *Client) FinishLogin(ctx context.Context, request *proto.LoginFinishRequest) (*proto.SessionResponse, error) {
	var response proto.SessionResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/auth/login/finish", nil, request, &response, nil)
}

func (c *Client) Logout(ctx context.Context) error {
	return c.sendRequest(ctx, "POST", "/api/v1/auth/logout", nil, nil, nil, nil)
}

func (c *Client) GetAccount(ctx context.Context) (*proto.Account, error) {
	var account proto.Account
	return &account, c.sendRequest(ctx, "GET", "/api/v1/account", nil, nil, &account, nil)
}

func (c *Client) ListAllMember(ctx context.Context) ([]proto.Member, error) {
	var allMember []proto.Member
	return allMember, c.sendRequest(ctx, "GET", "/api/v1/members", nil, nil, &allMember, nil)
}

func (c *Client) Invite(ctx context.Context, request *proto.InviteRequest) (*proto.InvitationSentResponse, error) {
	var response proto.InvitationSentResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/invites", nil, request, &response, nil)
}

func (c *Client) CreateShareInvitation(ctx context.Context, vaultID string, request *proto.ShareInvitationRequest) (*proto.InvitationSentResponse, error) {
	var response proto.InvitationSentResponse
	return &response, c.sendRequest(ctx, "POST", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/share-invitations", nil, request, &response, nil)
}

func (c *Client) ListAllShareInvitation(ctx context.Context) (*proto.ShareInvitationList, error) {
	var response proto.ShareInvitationList
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

func (c *Client) ListAllDevice(ctx context.Context) ([]proto.Device, error) {
	var allDevice []proto.Device
	return allDevice, c.sendRequest(ctx, "GET", "/api/v1/devices", nil, nil, &allDevice, nil)
}

func (c *Client) RevokeDevice(ctx context.Context, deviceID string) error {
	return c.sendRequest(ctx, "DELETE", "/api/v1/devices/"+url.PathEscape(deviceID), nil, nil, nil, nil)
}

func (c *Client) ListAllVault(ctx context.Context) ([]proto.Vault, error) {
	var allVault []proto.Vault
	return allVault, c.sendRequest(ctx, "GET", "/api/v1/vaults", nil, nil, &allVault, nil)
}

func (c *Client) CreateVault(ctx context.Context, request *proto.CreateVaultRequest) error {
	return c.sendRequest(ctx, "POST", "/api/v1/vaults", nil, request, nil, nil)
}

func (c *Client) ListAllVaultMember(ctx context.Context, vaultID string) ([]proto.VaultMember, error) {
	var allVaultMember []proto.VaultMember
	return allVaultMember, c.sendRequest(ctx, "GET", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/members", nil, nil, &allVaultMember, nil)
}

func (c *Client) SetAccess(ctx context.Context, vaultID, accountID string, request *proto.SetAccessRequest) error {
	return c.sendRequest(ctx, "PUT", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/access/"+url.PathEscape(accountID), nil, request, nil, nil)
}

func (c *Client) RemoveAccess(ctx context.Context, vaultID, accountID string) error {
	return c.sendRequest(ctx, "DELETE", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/access/"+url.PathEscape(accountID), nil, nil, nil, nil)
}

func (c *Client) RotateVault(ctx context.Context, vaultID string, request *proto.RotateVaultRequest) error {
	return c.sendRequest(ctx, "POST", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/rotate", nil, request, nil, nil)
}

func (c *Client) ListAllChange(ctx context.Context, vaultID string, since int64, limit int) (*proto.ChangesResponse, error) {
	var response proto.ChangesResponse
	query := url.Values{"since": {strconv.FormatInt(since, 10)}}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	return &response, c.sendRequest(ctx, "GET", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/changes?"+query.Encode(), nil, nil, &response, nil)
}

// PutItem returns the server's current item on a 409 (nil if the item does not exist).
func (c *Client) PutItem(ctx context.Context, vaultID, itemID, baseRevision string, request *proto.PutItemRequest) (int64, *proto.Item, error) {
	header := http.Header{}
	if baseRevision != "" {
		header.Set("If-Match", `"`+baseRevision+`"`)
	}
	var response proto.PutItemResponse
	var conflict proto.ConflictResponse
	err := c.sendRequest(ctx, "PUT", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/items/"+url.PathEscape(itemID), header, request, &response, &conflict)
	if IsStatus(err, http.StatusConflict) {
		return 0, conflict.Current, err
	}
	return response.Seq, nil, err
}

func (c *Client) CreateShareLink(ctx context.Context, request *proto.CreateShareLinkRequest) (*proto.ShareLink, error) {
	var response proto.ShareLink
	return &response, c.sendRequest(ctx, "POST", "/api/v1/share-links", nil, request, &response, nil)
}

func (c *Client) ListAllShareLink(ctx context.Context) ([]proto.ShareLink, error) {
	var allShareLink []proto.ShareLink
	return allShareLink, c.sendRequest(ctx, "GET", "/api/v1/share-links", nil, nil, &allShareLink, nil)
}

func (c *Client) RevokeShareLink(ctx context.Context, shareLinkID string) error {
	return c.sendRequest(ctx, "DELETE", "/api/v1/share-links/"+url.PathEscape(shareLinkID), nil, nil, nil, nil)
}

// OpenPublicShareLink needs no session and counts one view.
func (c *Client) OpenPublicShareLink(ctx context.Context, shareLinkID string) (*proto.PublicShareLink, error) {
	var response proto.PublicShareLink
	return &response, c.sendRequest(ctx, "GET", "/api/v1/public/share-links/"+url.PathEscape(shareLinkID), nil, nil, &response, nil)
}

func (c *Client) GetItemHistory(ctx context.Context, vaultID, itemID string) ([]proto.Item, error) {
	var allItem []proto.Item
	return allItem, c.sendRequest(ctx, "GET", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/items/"+url.PathEscape(itemID)+"/history", nil, nil, &allItem, nil)
}

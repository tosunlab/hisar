package client

import (
	"bytes"
	"context"
	"encoding/json"
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

func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("server: %s (%d)", e.Message, e.Status) }

func IsStatus(err error, status int) bool {
	e, ok := err.(*APIError)
	return ok && e.Status == status
}

// do sends a JSON request; for a 409 the body is decoded into conflict when non-nil.
func (c *Client) do(ctx context.Context, method, path string, hdr http.Header, in, out, conflict any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusConflict && conflict != nil {
		if err := json.Unmarshal(data, conflict); err != nil {
			return err
		}
		return &APIError{Status: resp.StatusCode, Message: "conflict"}
	}
	if resp.StatusCode >= 300 {
		var e proto.Error
		if json.Unmarshal(data, &e) != nil || e.Error == "" {
			e.Error = http.StatusText(resp.StatusCode)
		}
		return &APIError{Status: resp.StatusCode, Message: e.Error}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (c *Client) Prelogin(ctx context.Context, email string) (*proto.PreloginResponse, error) {
	var out proto.PreloginResponse
	return &out, c.do(ctx, "GET", "/api/v1/auth/prelogin?email="+url.QueryEscape(email), nil, nil, &out, nil)
}

func (c *Client) RegisterStart(ctx context.Context, req *proto.RegisterStartRequest) (*proto.RegisterStartResponse, error) {
	var out proto.RegisterStartResponse
	return &out, c.do(ctx, "POST", "/api/v1/auth/register/start", nil, req, &out, nil)
}

func (c *Client) RegisterFinish(ctx context.Context, req *proto.RegisterFinishRequest) (*proto.SessionResponse, error) {
	var out proto.SessionResponse
	return &out, c.do(ctx, "POST", "/api/v1/auth/register/finish", nil, req, &out, nil)
}

func (c *Client) LoginStart(ctx context.Context, req *proto.LoginStartRequest) (*proto.LoginStartResponse, error) {
	var out proto.LoginStartResponse
	return &out, c.do(ctx, "POST", "/api/v1/auth/login/start", nil, req, &out, nil)
}

func (c *Client) LoginFinish(ctx context.Context, req *proto.LoginFinishRequest) (*proto.SessionResponse, error) {
	var out proto.SessionResponse
	return &out, c.do(ctx, "POST", "/api/v1/auth/login/finish", nil, req, &out, nil)
}

func (c *Client) Logout(ctx context.Context) error {
	return c.do(ctx, "POST", "/api/v1/auth/logout", nil, nil, nil, nil)
}

func (c *Client) Account(ctx context.Context) (*proto.Account, error) {
	var out proto.Account
	return &out, c.do(ctx, "GET", "/api/v1/account", nil, nil, &out, nil)
}

func (c *Client) Members(ctx context.Context) ([]proto.Member, error) {
	var out []proto.Member
	return out, c.do(ctx, "GET", "/api/v1/members", nil, nil, &out, nil)
}

func (c *Client) Invite(ctx context.Context, req *proto.InviteRequest) (*proto.InviteResponse, error) {
	var out proto.InviteResponse
	return &out, c.do(ctx, "POST", "/api/v1/invites", nil, req, &out, nil)
}

func (c *Client) Devices(ctx context.Context) ([]proto.Device, error) {
	var out []proto.Device
	return out, c.do(ctx, "GET", "/api/v1/devices", nil, nil, &out, nil)
}

func (c *Client) RevokeDevice(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/v1/devices/"+url.PathEscape(id), nil, nil, nil, nil)
}

func (c *Client) Vaults(ctx context.Context) ([]proto.Vault, error) {
	var out []proto.Vault
	return out, c.do(ctx, "GET", "/api/v1/vaults", nil, nil, &out, nil)
}

func (c *Client) CreateVault(ctx context.Context, req *proto.CreateVaultRequest) error {
	return c.do(ctx, "POST", "/api/v1/vaults", nil, req, nil, nil)
}

func (c *Client) VaultMembers(ctx context.Context, vaultID string) ([]proto.VaultMember, error) {
	var out []proto.VaultMember
	return out, c.do(ctx, "GET", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/members", nil, nil, &out, nil)
}

func (c *Client) SetAccess(ctx context.Context, vaultID, accountID string, req *proto.SetAccessRequest) error {
	return c.do(ctx, "PUT", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/access/"+url.PathEscape(accountID), nil, req, nil, nil)
}

func (c *Client) RemoveAccess(ctx context.Context, vaultID, accountID string) error {
	return c.do(ctx, "DELETE", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/access/"+url.PathEscape(accountID), nil, nil, nil, nil)
}

func (c *Client) Changes(ctx context.Context, vaultID string, since int64, limit int) (*proto.ChangesResponse, error) {
	var out proto.ChangesResponse
	q := url.Values{"since": {strconv.FormatInt(since, 10)}}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return &out, c.do(ctx, "GET", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/changes?"+q.Encode(), nil, nil, &out, nil)
}

// PutItem returns the server's current item on a 409 (nil if the item does not exist).
func (c *Client) PutItem(ctx context.Context, vaultID, itemID, baseRevision string, req *proto.PutItemRequest) (int64, *proto.Item, error) {
	hdr := http.Header{}
	if baseRevision != "" {
		hdr.Set("If-Match", `"`+baseRevision+`"`)
	}
	var out proto.PutItemResponse
	var conflict proto.ConflictResponse
	err := c.do(ctx, "PUT", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/items/"+url.PathEscape(itemID), hdr, req, &out, &conflict)
	if IsStatus(err, http.StatusConflict) {
		return 0, conflict.Current, err
	}
	return out.Seq, nil, err
}

func (c *Client) ItemHistory(ctx context.Context, vaultID, itemID string) ([]proto.Item, error) {
	var out []proto.Item
	return out, c.do(ctx, "GET", "/api/v1/vaults/"+url.PathEscape(vaultID)+"/items/"+url.PathEscape(itemID)+"/history", nil, nil, &out, nil)
}

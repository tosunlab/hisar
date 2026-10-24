package apiclient

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/tryhisar/hisar/core/apitypes"
)

var ErrStagedVaultChangeUnsupported = errors.New("update the Hisar server to complete bounded vault key changes")

func transitionPath(vaultID, id string) string {
	path := "/api/v1/vaults/" + url.PathEscape(vaultID) + "/transitions"
	if id != "" {
		path += "/" + url.PathEscape(id)
	}
	return path
}
func (c *Client) RequireVaultTransitions(ctx context.Context, vaultID string) error {
	var result struct {
		Protocol int `json:"protocol"`
	}
	err := c.sendRequest(ctx, "GET", transitionPath(vaultID, ""), nil, nil, &result, nil)
	if IsStatus(err, http.StatusNotFound) || IsStatus(err, http.StatusMethodNotAllowed) {
		return ErrStagedVaultChangeUnsupported
	}
	if err != nil {
		return err
	}
	if result.Protocol != apitypes.VaultTransitionProtocol {
		return ErrStagedVaultChangeUnsupported
	}
	return nil
}

func (c *Client) BeginVaultTransition(ctx context.Context, vaultID string, request *apitypes.BeginVaultTransitionRequest) (*apitypes.VaultTransition, error) {
	var result apitypes.VaultTransition
	err := c.sendRequest(ctx, "POST", transitionPath(vaultID, ""), nil, request, &result, nil)
	if IsStatus(err, http.StatusNotFound) || IsStatus(err, http.StatusMethodNotAllowed) {
		return nil, ErrStagedVaultChangeUnsupported
	}
	if err != nil {
		return nil, err
	}
	if result.Protocol != apitypes.VaultTransitionProtocol || result.ID != request.ID || result.Kind != request.Kind || result.State != "staging" {
		return nil, ErrStagedVaultChangeUnsupported
	}
	return &result, nil
}

func (c *Client) GetVaultTransition(ctx context.Context, vaultID, id string) (*apitypes.VaultTransition, error) {
	var result apitypes.VaultTransition
	err := c.sendRequest(ctx, "GET", transitionPath(vaultID, id), nil, nil, &result, nil)
	if err == nil && (result.Protocol != apitypes.VaultTransitionProtocol || result.ID != id) {
		return nil, ErrStagedVaultChangeUnsupported
	}
	return &result, err
}

func (c *Client) ListVaultTransitionItems(ctx context.Context, vaultID, id, after string) (apitypes.VaultTransitionPage, error) {
	var result apitypes.VaultTransitionPage
	err := c.sendRequest(ctx, "GET", transitionPath(vaultID, id)+"/items?after="+url.QueryEscape(after), nil, nil, &result, nil)
	return result, err
}

func (c *Client) StageVaultItems(ctx context.Context, vaultID, id string, request *apitypes.StageVaultItemsRequest) error {
	return c.sendRequest(ctx, "POST", transitionPath(vaultID, id)+"/items", nil, request, nil, nil)
}
func (c *Client) StageVaultAccess(ctx context.Context, vaultID, id string, request *apitypes.StageVaultAccessRequest) error {
	return c.sendRequest(ctx, "POST", transitionPath(vaultID, id)+"/access", nil, request, nil, nil)
}
func (c *Client) CommitVaultTransition(ctx context.Context, vaultID, id string, request *apitypes.CommitVaultTransitionRequest) (*apitypes.VaultTransition, error) {
	var result apitypes.VaultTransition
	err := c.sendRequest(ctx, "POST", transitionPath(vaultID, id)+"/commit", nil, request, &result, nil)
	if err == nil && (result.Protocol != apitypes.VaultTransitionProtocol || result.ID != id || result.State != "committed") {
		return nil, ErrStagedVaultChangeUnsupported
	}
	return &result, err
}
func (c *Client) AbortVaultTransition(ctx context.Context, vaultID, id string) error {
	return c.sendRequest(ctx, "POST", transitionPath(vaultID, id)+"/abort", nil, nil, nil, nil)
}

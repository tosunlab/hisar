package apiclient

import (
	"context"
	"net/url"

	"github.com/tryhisar/hisar/core/apitypes"
)

func (c *Client) GetAdminAccountDeletionPreview(ctx context.Context, accountID string) (*apitypes.AccountDeletionPreview, error) {
	var response apitypes.AccountDeletionPreview
	if err := c.sendRequest(ctx, "GET", "/api/v1/admin/accounts/"+url.PathEscape(accountID)+"/deletion-preview", nil, nil, &response, nil); err != nil {
		return nil, err
	}
	return &response, nil
}

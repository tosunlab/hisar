package apiclient

import (
	"context"
	"net/url"

	"github.com/tryhisar/hisar/core/apitypes"
)

func (c *Client) GetAdminAccountDeletionPreview(ctx context.Context, accountID string) (*apitypes.AccountDeletionPreview, error) {
	var response apitypes.AccountDeletionPreview
	return &response, c.collectionRequest(ctx, "/api/v1/admin/accounts/"+url.PathEscape(accountID)+"/deletion-preview", &response)
}

package apiclient

import (
	"context"
	"net/url"

	"github.com/tryhisar/hisar/core/apitypes"
)

func (c *Client) CreateApproval(ctx context.Context, request *apitypes.CreateApprovalRequest) error {
	return c.sendRequest(ctx, "POST", "/api/v1/approvals", nil, request, nil, nil)
}

func (c *Client) ListAllPendingApproval(ctx context.Context) ([]apitypes.PendingApproval, error) {
	var allPending []apitypes.PendingApproval
	return allPending, c.sendRequest(ctx, "GET", "/api/v1/approvals", nil, nil, &allPending, nil)
}

func (c *Client) GetApproval(ctx context.Context, requestID string) (*apitypes.Approval, error) {
	var approval apitypes.Approval
	return &approval, c.sendRequest(ctx, "GET", "/api/v1/approvals/"+url.PathEscape(requestID), nil, nil, &approval, nil)
}

func (c *Client) AnswerApproval(ctx context.Context, requestID string, sealedAnswer []byte) error {
	return c.sendRequest(ctx, "POST", "/api/v1/approvals/"+url.PathEscape(requestID)+"/answer", nil, &apitypes.AnswerApprovalRequest{SealedAnswer: sealedAnswer}, nil, nil)
}

func (c *Client) ResolveApproval(ctx context.Context, requestID string) error {
	return c.sendRequest(ctx, "DELETE", "/api/v1/approvals/"+url.PathEscape(requestID), nil, nil, nil, nil)
}

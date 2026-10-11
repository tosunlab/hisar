package client

import (
	"context"
	"net/url"

	"github.com/tryhisar/hisar/core/proto"
)

func (c *Client) CreateApproval(ctx context.Context, request *proto.CreateApprovalRequest) error {
	return c.sendRequest(ctx, "POST", "/api/v1/approvals", nil, request, nil, nil)
}

func (c *Client) ListAllPendingApproval(ctx context.Context) ([]proto.PendingApproval, error) {
	var allPending []proto.PendingApproval
	return allPending, c.sendRequest(ctx, "GET", "/api/v1/approvals", nil, nil, &allPending, nil)
}

func (c *Client) GetApproval(ctx context.Context, requestID string) (*proto.Approval, error) {
	var approval proto.Approval
	return &approval, c.sendRequest(ctx, "GET", "/api/v1/approvals/"+url.PathEscape(requestID), nil, nil, &approval, nil)
}

func (c *Client) AnswerApproval(ctx context.Context, requestID string, sealedAnswer []byte) error {
	return c.sendRequest(ctx, "POST", "/api/v1/approvals/"+url.PathEscape(requestID)+"/answer", nil, &proto.AnswerApprovalRequest{SealedAnswer: sealedAnswer}, nil, nil)
}

func (c *Client) ResolveApproval(ctx context.Context, requestID string) error {
	return c.sendRequest(ctx, "DELETE", "/api/v1/approvals/"+url.PathEscape(requestID), nil, nil, nil, nil)
}

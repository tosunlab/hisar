package session

import (
	"context"
	"errors"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/sharelink"
)

type ShareLinkOptions struct {
	Expiry     time.Duration
	MaxViews   int
	Passphrase []byte
}

// ShareLink is a link as its creator sees it; URL is empty when the label no longer opens.
type ShareLink struct {
	ID            string
	URL           string
	Title         string
	VaultID       string
	ItemID        string
	FieldLabels   []string
	IsText        bool
	CreatedAt     time.Time
	ExpiresAt     time.Time
	MaxViews      int
	ViewCount     int
	HasPassphrase bool
}

func (options ShareLinkOptions) validate() error {
	if !sharelink.IsAllowedExpiry(options.Expiry) {
		return errors.New("choose an expiry of 1 hour, 1 day, 7 days or 30 days")
	}
	if !sharelink.IsAllowedViews(options.MaxViews) {
		return errors.New("choose 1 to 100 views, or unlimited")
	}
	return nil
}

func (s *Session) ShareItem(ctx context.Context, vault *Vault, vaultItem *item.Item, fieldKeys []string, options ShareLinkOptions) (*ShareLink, error) {
	payload, err := sharelink.BuildItemPayload(vaultItem, fieldKeys)
	if err != nil {
		return nil, err
	}
	label := &sharelink.Label{Title: payload.Title, VaultID: vault.ID, ItemID: vaultItem.ID, FieldLabels: []string{}}
	for _, key := range fieldKeys {
		label.FieldLabels = append(label.FieldLabels, sharelink.DescribeFieldKey(vaultItem, key))
	}
	return s.createShareLink(ctx, payload, label, options)
}

func (s *Session) ShareText(ctx context.Context, title, text string, options ShareLinkOptions) (*ShareLink, error) {
	payload, err := sharelink.BuildTextPayload(title, text)
	if err != nil {
		return nil, err
	}
	return s.createShareLink(ctx, payload, &sharelink.Label{Title: payload.Title, FieldLabels: []string{"text"}, IsText: true}, options)
}

func (s *Session) createShareLink(ctx context.Context, payload *sharelink.Payload, label *sharelink.Label, options ShareLinkOptions) (*ShareLink, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	sealed, err := sharelink.Seal(payload, options.Passphrase)
	if err != nil {
		return nil, err
	}
	label.LinkKey = sealed.LinkKey
	encryptedLabel, err := sharelink.SealLabel(s.Keyset.UserKey, s.AccountID, label)
	if err != nil {
		return nil, err
	}
	request := &apitypes.CreateShareLinkRequest{
		Ciphertext:       sealed.Ciphertext,
		EncryptedLabel:   encryptedLabel,
		ExpiresInSeconds: int64(options.Expiry / time.Second),
		MaxViews:         options.MaxViews,
	}
	if sealed.Passphrase != nil {
		request.Passphrase = &apitypes.ShareLinkPassphrase{Salt: sealed.Passphrase.Salt, Iterations: sealed.Passphrase.Iterations}
	}
	created, err := s.Client.CreateShareLink(ctx, request)
	if err != nil {
		return nil, err
	}
	return s.buildShareLink(created, label), nil
}

func (s *Session) buildShareLink(link *apitypes.ShareLink, label *sharelink.Label) *ShareLink {
	shareLink := &ShareLink{
		ID: link.ID, CreatedAt: link.CreatedAt, ExpiresAt: link.ExpiresAt, MaxViews: link.MaxViews, ViewCount: link.ViewCount,
		HasPassphrase: link.HasPassphrase, Title: "Unknown link", FieldLabels: []string{},
	}
	if label != nil {
		shareLink.Title, shareLink.VaultID, shareLink.ItemID, shareLink.IsText = label.Title, label.VaultID, label.ItemID, label.IsText
		shareLink.FieldLabels = append(shareLink.FieldLabels, label.FieldLabels...)
		if len(label.LinkKey) > 0 {
			shareLink.URL = sharelink.BuildURL(s.Client.BaseURL, link.ID, label.LinkKey)
		}
	}
	return shareLink
}

func (s *Session) ListShareLinks(ctx context.Context) ([]ShareLink, error) {
	links, err := s.Client.ListShareLinks(ctx)
	if err != nil {
		return nil, err
	}
	shareLinks := make([]ShareLink, 0, len(links))
	for index := range links {
		label, err := sharelink.OpenLabel(s.Keyset.UserKey, s.AccountID, links[index].EncryptedLabel)
		if err != nil {
			label = nil
		}
		shareLinks = append(shareLinks, *s.buildShareLink(&links[index], label))
	}
	return shareLinks, nil
}

func (s *Session) RevokeShareLink(ctx context.Context, shareLinkID string) error {
	return s.Client.RevokeShareLink(ctx, shareLinkID)
}

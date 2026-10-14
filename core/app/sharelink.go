package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/sharelink"
)

// ShareLinkField is a field the share dialog offers; Key goes back in ShareLinkRequest.FieldKeys.
type ShareLinkField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	IsDefault   bool   `json:"isDefault"`
	IsConcealed bool   `json:"isConcealed"`
}

// ShareLinkRequest shares FieldKeys of an item, or Text with Title when ItemID is empty; MaxViews 0 is unlimited.
type ShareLinkRequest struct {
	VaultID          string   `json:"vaultID"`
	ItemID           string   `json:"itemID"`
	FieldKeys        []string `json:"fieldKeys"`
	Title            string   `json:"title"`
	Text             string   `json:"text"`
	ExpiresInSeconds int      `json:"expiresInSeconds"`
	MaxViews         int      `json:"maxViews"`
	Passphrase       string   `json:"passphrase"`
}

type ShareLink struct {
	ID            string   `json:"id"`
	URL           string   `json:"url"`
	Title         string   `json:"title"`
	VaultID       string   `json:"vaultID"`
	ItemID        string   `json:"itemID"`
	FieldLabels   []string `json:"fieldLabels"`
	IsText        bool     `json:"isText"`
	CreatedAt     string   `json:"createdAt"`
	ExpiresAt     string   `json:"expiresAt"`
	MaxViews      int      `json:"maxViews"`
	ViewCount     int      `json:"viewCount"`
	HasPassphrase bool     `json:"hasPassphrase"`
}

var errShareLinkGone = errors.New("this link was already revoked, used up or expired")

func convertToShareLink(link *session.ShareLink) ShareLink {
	return ShareLink{
		ID: link.ID, URL: link.URL, Title: link.Title, VaultID: link.VaultID, ItemID: link.ItemID,
		FieldLabels: append([]string{}, link.FieldLabels...), IsText: link.IsText,
		CreatedAt: link.CreatedAt.UTC().Format(time.RFC3339), ExpiresAt: link.ExpiresAt.UTC().Format(time.RFC3339),
		MaxViews: link.MaxViews, ViewCount: link.ViewCount, HasPassphrase: link.HasPassphrase,
	}
}

func listAllShareLinkField(vaultItem *item.Item) []ShareLinkField {
	allDefaultKey := sharelink.ListAllDefaultFieldKey(vaultItem)
	allField := []ShareLinkField{}
	for _, key := range sharelink.ListAllAvailableFieldKey(vaultItem) {
		field := ShareLinkField{Key: key, Label: sharelink.DescribeFieldKey(vaultItem, key), IsDefault: slices.Contains(allDefaultKey, key)}
		switch key {
		case sharelink.FieldPassword:
			field.IsConcealed = true
		case sharelink.FieldUsername, sharelink.FieldURLs, sharelink.FieldNotes:
		default:
			for index, itemField := range vaultItem.Details.Fields {
				if sharelink.GetCustomFieldKey(index) == key {
					field.IsConcealed = itemField.Concealed
				}
			}
		}
		allField = append(allField, field)
	}
	return allField
}

func (s *Service) ListAllShareLinkField(vaultID, itemID string) ([]ShareLinkField, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	vaultItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return nil, err
	}
	return listAllShareLinkField(vaultItem), nil
}

func (request ShareLinkRequest) buildOptions() session.ShareLinkOptions {
	return session.ShareLinkOptions{Expiry: time.Duration(request.ExpiresInSeconds) * time.Second, MaxViews: request.MaxViews, Passphrase: []byte(request.Passphrase)}
}

func (s *Service) CreateShareLink(request ShareLinkRequest) (ShareLink, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ShareLink{}, err
	}
	options := request.buildOptions()
	defer clear(options.Passphrase)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var created *session.ShareLink
	if request.ItemID == "" {
		var err error
		if created, err = s.unlockedSession.ShareText(ctx, request.Title, request.Text, options); err != nil {
			return ShareLink{}, convertError(err)
		}
		return convertToShareLink(created), nil
	}
	vault, err := s.findVault(ctx, request.VaultID)
	if err != nil {
		return ShareLink{}, err
	}
	vaultItem, err := s.findItem(request.VaultID, request.ItemID)
	if err != nil {
		return ShareLink{}, err
	}
	if created, err = s.unlockedSession.ShareItem(ctx, vault, vaultItem, request.FieldKeys, options); err != nil {
		return ShareLink{}, convertError(err)
	}
	return convertToShareLink(created), nil
}

func (s *Service) ListAllShareLink() ([]ShareLink, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	allLink, err := s.unlockedSession.ListAllShareLink(ctx)
	if err != nil {
		return nil, convertError(err)
	}
	allShareLink := make([]ShareLink, 0, len(allLink))
	for index := range allLink {
		shareLink := convertToShareLink(&allLink[index])
		// Lists never carry the key; CopyShareLink copies a link through the backend.
		shareLink.URL = ""
		allShareLink = append(allShareLink, shareLink)
	}
	return allShareLink, nil
}

func (s *Service) RevokeShareLink(shareLinkID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := s.unlockedSession.RevokeShareLink(ctx, shareLinkID)
	if apiclient.IsStatus(err, http.StatusNotFound) {
		return errShareLinkGone
	}
	return convertError(err)
}

// CopyShareLink copies a live link again; the clipboard is cleared like any copied secret.
func (s *Service) CopyShareLink(shareLinkID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	allLink, err := s.unlockedSession.ListAllShareLink(ctx)
	if err != nil {
		return convertError(err)
	}
	index := slices.IndexFunc(allLink, func(link session.ShareLink) bool { return link.ID == shareLinkID })
	if index < 0 {
		return errShareLinkGone
	}
	if allLink[index].URL == "" {
		return fmt.Errorf("this link %w on this device", ErrNotFound)
	}
	return s.copyText(allLink[index].URL)
}

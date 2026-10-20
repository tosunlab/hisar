package api

import (
	"encoding/json"
	"github.com/gofiber/fiber/v3"
	"github.com/tryhisar/hisar/core/apitypes"
	"strings"
)

func legacyCollectionTarget(path string) any {
	switch path {
	case "/members":
		return &[]apitypes.Member{}
	case "/devices":
		return &[]apitypes.Device{}
	case "/vaults":
		return &[]apitypes.Vault{}
	case "/share-links":
		return &[]apitypes.ShareLink{}
	case "/organizations":
		return &apitypes.OrganizationList{Organizations: []apitypes.OrganizationSummary{}, Invitations: []apitypes.ReceivedOrganizationInvitation{}}
	case "/share-invitations":
		return &apitypes.ShareInvitationList{Received: []apitypes.ReceivedShareInvitation{}, Sent: []apitypes.SentShareInvitation{}, Waiting: []apitypes.WaitingShareInvitation{}}
	case "/vault-key-requests":
		return &apitypes.VaultKeyRequestList{Pending: []apitypes.VaultKeyRequest{}, Waiting: []apitypes.WaitingVaultKey{}}
	case "/admin/invites":
		return &apitypes.AdminInviteList{Invites: []apitypes.AdminInvite{}}
	}
	if strings.HasSuffix(path, "/history") {
		return &[]apitypes.Item{}
	}
	if strings.HasSuffix(path, "/members") {
		return &[]apitypes.VaultMember{}
	}
	if strings.HasPrefix(path, "/admin/accounts/") {
		return &apitypes.AdminAccountDetail{Devices: []apitypes.AdminDevice{}}
	}
	return &apitypes.Organization{Members: []apitypes.OrganizationMember{}, Teams: []apitypes.Team{}, Invitations: []apitypes.SentOrganizationInvitation{}}
}

func (s *Server) sendCollection(c fiber.Ctx) error {
	path := strings.TrimPrefix(c.Path(), "/api/v1")
	version := c.Get(apitypes.CollectionPageHeader)
	if version != "" && version != apitypes.CollectionPageVersion {
		return createAPIError(fiber.StatusPreconditionRequired, "update this app to use supported collection pagination")
	}
	if version != "" {
		page, err := s.service.ReadCollectionPage(c.Context(), s.getActor(c), path, c.Query("cursor"))
		if err != nil {
			return err
		}
		c.Set(apitypes.CollectionPageHeader, apitypes.CollectionPageVersion)
		return c.JSON(page)
	}
	if c.Query("cursor") != "" {
		return createAPIError(fiber.StatusBadRequest, "collection cursors require the pagination feature header")
	}
	// Old peers receive a complete bounded result or an actionable refusal, never a first page.
	target := legacyCollectionTarget(path)
	assembler := apitypes.NewCollectionAssembler(target)
	cursor := ""
	bytes := 0
	for {
		page, err := s.service.ReadCollectionPage(c.Context(), s.getActor(c), path, cursor)
		if err != nil {
			return err
		}
		for _, record := range page.Records {
			bytes += len(record.Value) + len(record.Key) + len(record.Parent) + len(record.Field) + 128
			if bytes > apitypes.CollectionPageBytes {
				return createAPIError(fiber.StatusPreconditionRequired, "update this app to read this collection with pagination; no records were omitted")
			}
			if err := assembler.Apply(record); err != nil {
				return err
			}
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	assembler.Finish()
	data, err := json.Marshal(target)
	if err != nil {
		return err
	}
	if len(data) > apitypes.CollectionPageBytes {
		return createAPIError(fiber.StatusPreconditionRequired, "update this app to read this collection with pagination; no records were omitted")
	}
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	return c.Send(data)
}

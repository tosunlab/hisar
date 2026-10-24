package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/tryhisar/hisar/core/apitypes"
)

type deletionPageCursor struct {
	Scope    string `json:"s"`
	Snapshot string `json:"v"`
	After    string `json:"a"`
}

// sendDeletionPreview pages the exact preview from the canonical plan builder. It does not replace that builder.
func (s *Server) sendDeletionPreview(c fiber.Ctx, preview apitypes.AccountDeletionPreview) error {
	version := c.Get(apitypes.CollectionPageHeader)
	if version != "" && version != apitypes.CollectionPageVersion {
		return createAPIError(fiber.StatusPreconditionRequired, "update this app to use supported collection pagination")
	}
	slices.SortFunc(preview.OwnedVaults, func(a, b apitypes.DeletionOwnedVault) int { return strings.Compare(a.VaultID, b.VaultID) })
	slices.SortFunc(preview.MemberVaults, func(a, b apitypes.DeletionMemberVault) int { return strings.Compare(a.VaultID, b.VaultID) })
	slices.SortFunc(preview.Organizations, func(a, b apitypes.DeletionOrganization) int {
		return strings.Compare(a.OrganizationID, b.OrganizationID)
	})
	header := preview
	header.OwnedVaults = []apitypes.DeletionOwnedVault{}
	header.MemberVaults = []apitypes.DeletionMemberVault{}
	header.Organizations = []apitypes.DeletionOrganization{}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return err
	}
	var plan struct {
		PlanVersion string `json:"planVersion"`
	}
	if err := json.Unmarshal(headerBytes, &plan); err != nil {
		return err
	}
	count := 1 + len(preview.OwnedVaults) + len(preview.MemberVaults) + len(preview.Organizations)
	recordAt := func(index int) (apitypes.CollectionRecord, error) {
		if index == 0 {
			return apitypes.CollectionRecord{Key: "0", Field: "$", Value: headerBytes}, nil
		}
		index--
		var field, key string
		var value any
		switch {
		case index < len(preview.OwnedVaults):
			field = "ownedVaults"
			key = "1/" + preview.OwnedVaults[index].VaultID
			value = preview.OwnedVaults[index]
		case index < len(preview.OwnedVaults)+len(preview.MemberVaults):
			index -= len(preview.OwnedVaults)
			field = "memberVaults"
			key = "2/" + preview.MemberVaults[index].VaultID
			value = preview.MemberVaults[index]
		default:
			index -= len(preview.OwnedVaults) + len(preview.MemberVaults)
			field = "organizations"
			key = "3/" + preview.Organizations[index].OrganizationID
			value = preview.Organizations[index]
		}
		data, err := json.Marshal(value)
		return apitypes.CollectionRecord{Key: key, Field: field, Value: data}, err
	}
	hash := sha256.New()
	bytes := 0
	for index := range count {
		record, err := recordAt(index)
		if err != nil {
			return err
		}
		hash.Write([]byte(record.Field))
		hash.Write([]byte{0})
		hash.Write(record.Value)
		hash.Write([]byte{0})
		bytes += len(record.Value) + len(record.Key) + len(record.Field) + 128
	}
	scope := s.getActor(c).AccountID + "/" + s.getActor(c).DeviceID + c.Path()
	snapshot := hex.EncodeToString(hash.Sum(nil))
	cursor := deletionPageCursor{Scope: scope, Snapshot: snapshot}
	if encoded := c.Query("cursor"); encoded != "" {
		if version == "" || len(encoded) > 4096 {
			return createAPIError(fiber.StatusBadRequest, "invalid deletion preview cursor")
		}
		data, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Scope != scope {
			return createAPIError(fiber.StatusBadRequest, "invalid deletion preview cursor")
		}
		if cursor.Snapshot != snapshot {
			return createAPIError(fiber.StatusConflict, apitypes.CollectionChangedMessage)
		}
	}
	if version == "" {
		if bytes > apitypes.CollectionPageBytes {
			return createAPIError(fiber.StatusPreconditionRequired, "update this app to review the complete account deletion preview with pagination")
		}
		return c.JSON(preview)
	}
	page := apitypes.CollectionPage{Version: apitypes.CollectionPageVersion, Records: []apitypes.CollectionRecord{}, Snapshot: snapshot, PlanVersion: plan.PlanVersion}
	bytes = 0
	for index := range count {
		record, err := recordAt(index)
		if err != nil {
			return err
		}
		if record.Key <= cursor.After {
			continue
		}
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if len(page.Records) >= apitypes.CollectionPageRows || bytes+len(data) > apitypes.CollectionPageBytes-4096 {
			encoded, _ := json.Marshal(cursor)
			page.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
			break
		}
		bytes += len(data)
		page.Records = append(page.Records, record)
		cursor.After = record.Key
	}
	c.Set(apitypes.CollectionPageHeader, apitypes.CollectionPageVersion)
	return c.JSON(page)
}

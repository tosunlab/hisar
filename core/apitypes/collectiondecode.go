package apitypes

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

func appendCollectionValue[T any](target *[]T, value json.RawMessage) error {
	var entry T
	if err := json.Unmarshal(value, &entry); err != nil {
		return err
	}
	*target = append(*target, entry)
	return nil
}

// CollectionAssembler owns one complete traversal. Team rows precede their independently paged edges.
type CollectionAssembler struct {
	target    any
	teamsByID map[string]int
}

func NewCollectionAssembler(target any) *CollectionAssembler {
	return &CollectionAssembler{target: target}
}
func (assembler *CollectionAssembler) Apply(r CollectionRecord) error {
	switch out := assembler.target.(type) {
	case *[]Member:
		return appendCollectionValue(out, r.Value)
	case *[]Device:
		return appendCollectionValue(out, r.Value)
	case *[]Vault:
		return appendCollectionValue(out, r.Value)
	case *[]VaultMember:
		return appendCollectionValue(out, r.Value)
	case *[]Item:
		return appendCollectionValue(out, r.Value)
	case *[]ShareLink:
		return appendCollectionValue(out, r.Value)
	case *AdminInviteList:
		if r.Field == "invites" {
			return appendCollectionValue(&out.Invites, r.Value)
		}
	case *AdminAccountDetail:
		switch r.Field {
		case "account":
			return json.Unmarshal(r.Value, &out.Account)
		case "devices":
			return appendCollectionValue(&out.Devices, r.Value)
		}
	case *AccountDeletionPreview:
		switch r.Field {
		case "$":
			return json.Unmarshal(r.Value, out)
		case "ownedVaults":
			return appendCollectionValue(&out.OwnedVaults, r.Value)
		case "memberVaults":
			return appendCollectionValue(&out.MemberVaults, r.Value)
		case "organizations":
			return appendCollectionValue(&out.Organizations, r.Value)
		}
	case *OrganizationList:
		switch r.Field {
		case "organizations":
			return appendCollectionValue(&out.Organizations, r.Value)
		case "invitations":
			return appendCollectionValue(&out.Invitations, r.Value)
		}
	case *ShareInvitationList:
		switch r.Field {
		case "received":
			return appendCollectionValue(&out.Received, r.Value)
		case "sent":
			return appendCollectionValue(&out.Sent, r.Value)
		case "waiting":
			return appendCollectionValue(&out.Waiting, r.Value)
		}
	case *VaultKeyRequestList:
		switch r.Field {
		case "pending":
			return appendCollectionValue(&out.Pending, r.Value)
		case "waiting":
			return appendCollectionValue(&out.Waiting, r.Value)
		}
	case *Organization:
		switch r.Field {
		case "$":
			return json.Unmarshal(r.Value, out)
		case "members":
			return appendCollectionValue(&out.Members, r.Value)
		case "teams":
			var team Team
			if err := json.Unmarshal(r.Value, &team); err != nil {
				return err
			}
			if assembler.teamsByID == nil {
				assembler.teamsByID = make(map[string]int)
			}
			if _, duplicate := assembler.teamsByID[team.ID]; duplicate {
				return errors.New("duplicate collection team")
			}
			assembler.teamsByID[team.ID] = len(out.Teams)
			out.Teams = append(out.Teams, team)
			return nil
		case "invitations":
			return appendCollectionValue(&out.Invitations, r.Value)
		case "memberIds", "vaults":
			index, found := assembler.teamsByID[r.Parent]
			if found {
				if r.Field == "memberIds" {
					return appendCollectionValue(&out.Teams[index].MemberIDs, r.Value)
				}
				return appendCollectionValue(&out.Teams[index].Vaults, r.Value)
			}
		}
	}
	return errors.New("unsupported collection record")
}

// Finish restores presentation order after transport pages have been assembled.
func (assembler *CollectionAssembler) Finish() {
	switch out := assembler.target.(type) {
	case *[]Item:
		slices.SortStableFunc(*out, func(a, b Item) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	case *[]ShareLink:
		slices.SortFunc(*out, func(a, b ShareLink) int {
			if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
				return order
			}
			return strings.Compare(a.ID, b.ID)
		})
	case *AdminInviteList:
		slices.SortFunc(out.Invites, func(a, b AdminInvite) int {
			if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
				return order
			}
			return strings.Compare(a.ID, b.ID)
		})
	case *AdminAccountDetail:
		slices.SortStableFunc(out.Devices, func(a, b AdminDevice) int {
			if (a.RevokedAt == nil) != (b.RevokedAt == nil) {
				if a.RevokedAt == nil {
					return -1
				}
				return 1
			}
			return b.LastSeenAt.Compare(a.LastSeenAt)
		})
	}
}

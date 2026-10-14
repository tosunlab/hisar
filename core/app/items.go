package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/totp"
)

// ListAllItem fetches the vault's items from the server and returns them without secrets in a stable order.
func (s *Service) ListAllItem(vaultID string) ([]ItemDetail, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	allItem, err := s.unlockedSession.ListAllItem(ctx, vault)
	if err != nil {
		return nil, convertError(err)
	}
	s.allItemByVaultID[vaultID] = allItem
	allSummary := make([]ItemDetail, 0, len(allItem))
	for index := range allItem {
		summary := convertToItemDetail(&allItem[index])
		summary.Password, summary.Notes, summary.Fields, summary.OneTimePassword = "", "", nil, ""
		allSummary = append(allSummary, summary)
	}
	return allSummary, nil
}

func (s *Service) findItem(vaultID, itemID string) (*item.Item, error) {
	for index, vaultItem := range s.allItemByVaultID[vaultID] {
		if vaultItem.ID == itemID {
			return &s.allItemByVaultID[vaultID][index], nil
		}
	}
	return nil, fmt.Errorf("item %w", ErrNotFound)
}

func (s *Service) GetItem(vaultID, itemID string) (ItemDetail, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ItemDetail{}, err
	}
	vaultItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return ItemDetail{}, err
	}
	return convertToItemDetail(vaultItem), nil
}

// GetOneTimeCode returns the item's current one-time code; the UI asks again once ExpiresAt passes.
func (s *Service) GetOneTimeCode(vaultID, itemID string) (OneTimeCode, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return OneTimeCode{}, err
	}
	vaultItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return OneTimeCode{}, err
	}
	return generateOneTimeCode(vaultItem, s.config.Now())
}

func generateOneTimeCode(vaultItem *item.Item, now time.Time) (OneTimeCode, error) {
	config, err := totp.Parse(vaultItem.Details.OneTimePassword)
	if err != nil {
		return OneTimeCode{}, fmt.Errorf("one-time password %w", ErrNotFound)
	}
	code, expiresAt := config.GenerateCode(now)
	return OneTimeCode{Code: code, ExpiresAt: expiresAt.UTC().Format(time.RFC3339), Period: config.Period}, nil
}

func convertToItemDetail(vaultItem *item.Item) ItemDetail {
	detail := ItemDetail{
		ID:          vaultItem.ID,
		Revision:    vaultItem.Revision,
		Category:    vaultItem.Overview.Category,
		Title:       vaultItem.Overview.Title,
		Username:    vaultItem.Overview.Username,
		URLs:        append([]string{}, vaultItem.Overview.URLs...),
		Password:    vaultItem.Details.Password,
		Notes:       vaultItem.Details.Notes,
		Fields:      []ItemField{},
		PublicKey:   vaultItem.Overview.PublicKey,
		Fingerprint: vaultItem.Overview.Fingerprint,
		KeyType:     vaultItem.Overview.KeyType,

		OneTimePassword:    vaultItem.Details.OneTimePassword,
		HasOneTimePassword: vaultItem.Details.OneTimePassword != "",

		AllPasskey: convertToAllItemPasskey(vaultItem.Details.Passkeys),
	}
	for _, field := range vaultItem.Details.Fields {
		detail.Fields = append(detail.Fields, ItemField{Name: field.Name, Value: field.Value, Concealed: field.Concealed})
	}
	return detail
}

func convertFromItemDetail(detail ItemDetail) item.Item {
	vaultItem := item.Item{
		ID:       detail.ID,
		Revision: detail.Revision,
		Overview: item.Overview{Title: strings.TrimSpace(detail.Title), Username: strings.TrimSpace(detail.Username)},
		Details:  item.Details{Password: detail.Password, Notes: detail.Notes, OneTimePassword: detail.OneTimePassword},
	}
	for _, link := range detail.URLs {
		if link = strings.TrimSpace(link); link != "" {
			vaultItem.Overview.URLs = append(vaultItem.Overview.URLs, link)
		}
	}
	for _, field := range detail.Fields {
		if strings.TrimSpace(field.Name) != "" {
			vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: strings.TrimSpace(field.Name), Value: field.Value, Concealed: field.Concealed})
		}
	}
	return vaultItem
}

// SaveItem creates the item when detail.ID is empty, otherwise updates it based on detail.Revision.
func (s *Service) SaveItem(vaultID string, detail ItemDetail) (ItemDetail, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ItemDetail{}, err
	}
	if strings.TrimSpace(detail.Title) == "" {
		return ItemDetail{}, errors.New("enter a title")
	}
	if strings.TrimSpace(detail.OneTimePassword) != "" {
		normalized, err := totp.Normalize(detail.OneTimePassword)
		if err != nil {
			return ItemDetail{}, errors.New("the one-time password is not a valid secret or otpauth:// link")
		}
		detail.OneTimePassword = normalized
	} else {
		detail.OneTimePassword = ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return ItemDetail{}, err
	}
	vaultItem, err := s.buildSavedItem(vaultID, detail)
	if err != nil {
		return ItemDetail{}, err
	}
	if _, err := s.unlockedSession.SaveItem(ctx, vault, &vaultItem); err != nil {
		return ItemDetail{}, convertError(err)
	}
	s.replaceCachedItem(vaultID, vaultItem)
	return convertToItemDetail(&vaultItem), nil
}

// buildSavedItem keeps the category and passkeys of an existing item; an ssh-key item only takes the title and notes from the editor.
func (s *Service) buildSavedItem(vaultID string, detail ItemDetail) (item.Item, error) {
	var cachedItem *item.Item
	if detail.ID != "" {
		cachedItem, _ = s.findItem(vaultID, detail.ID)
	}
	if cachedItem == nil {
		if detail.Category == item.CategorySSHKey {
			if detail.ID == "" {
				return item.Item{}, errors.New("generate or import an SSH key instead")
			}
			return item.Item{}, errors.New("this SSH key is not loaded; reload the vault and try again")
		}
		return convertFromItemDetail(detail), nil
	}
	if cachedItem.Overview.Category != item.CategorySSHKey {
		vaultItem := convertFromItemDetail(detail)
		vaultItem.Overview.Category = cachedItem.Overview.Category
		vaultItem.Details.Passkeys = cachedItem.Details.Passkeys
		item.UpdatePasskeyOverview(&vaultItem)
		return vaultItem, nil
	}
	vaultItem := *cachedItem
	vaultItem.Revision = detail.Revision
	vaultItem.Overview.Title = strings.TrimSpace(detail.Title)
	vaultItem.Details.Notes = detail.Notes
	vaultItem.Details.OneTimePassword = detail.OneTimePassword
	return vaultItem, nil
}

func (s *Service) replaceCachedItem(vaultID string, vaultItem item.Item) {
	allItem := s.allItemByVaultID[vaultID]
	for index := range allItem {
		if allItem[index].ID == vaultItem.ID {
			allItem[index] = vaultItem
			return
		}
	}
	s.allItemByVaultID[vaultID] = append(allItem, vaultItem)
}

func (s *Service) DeleteItem(vaultID, itemID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	vaultItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return err
	}
	if _, err := s.unlockedSession.DeleteItem(ctx, vault, vaultItem); err != nil {
		return convertError(err)
	}
	allRemaining := s.allItemByVaultID[vaultID][:0]
	for _, cachedItem := range s.allItemByVaultID[vaultID] {
		if cachedItem.ID != itemID {
			allRemaining = append(allRemaining, cachedItem)
		}
	}
	s.allItemByVaultID[vaultID] = allRemaining
	return nil
}

func (s *Service) GeneratePassword(config PasswordConfig) (string, error) {
	if config.Length == 0 {
		config.Length = item.DefaultGeneratorConfig().Length
	}
	return item.GeneratePassword(item.GeneratorConfig{Length: config.Length, Digits: config.Digits, Symbols: config.Symbols})
}

// fieldNames are the names the apps pass to CopyField for built-in fields.
var fieldNames = item.FieldNames{
	"username": item.FieldUsername, "password": item.FieldPassword, "url": item.FieldURL, "notes": item.FieldNotes,
	"public key": item.FieldPublicKey, "fingerprint": item.FieldFingerprint, "one-time code": item.FieldOneTimeCode,
}

func getFieldValue(vaultItem *item.Item, fieldName string) (string, bool) {
	return item.LookupField(vaultItem, fieldName, fieldNames, time.Now())
}

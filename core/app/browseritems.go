package app

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
)

// The extension's popup browses every vault (browserprotocol.FeatureVault). Lists and items.get carry no secret: a
// secret field's value leaves the app only in items.reveal, one field at a time, when the user asks for it. The
// extension answers these only to its own pages, never to a content script.

// listBrowserItems returns the items of every vault that match query (titles, usernames, websites and what the app's
// search reads), sorted by title.
func (s *Service) listBrowserItems(ctx context.Context, query, category string, isFavoriteOnly bool) ([]browserprotocol.ItemSummary, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, err
	}
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		return nil, err
	}
	s.loadFavoritesOnce(ctx)
	query = strings.ToLower(strings.TrimSpace(query))
	summaries := []browserprotocol.ItemSummary{}
	for vaultID, items := range s.itemsByVaultID {
		vault := s.vaultsByID[vaultID]
		if vault == nil {
			continue
		}
		for index := range items {
			vaultItem := &items[index]
			if category != "" && !vaultItem.IsCategory(category) {
				continue
			}
			if isFavoriteOnly && !s.favorites.IsFavorite(vaultItem.ID) {
				continue
			}
			if !isAgentSearchMatch(vault.Name, vaultItem, query) {
				continue
			}
			summaries = append(summaries, s.summarizeBrowserItem(vaultID, vault.Name, vaultItem))
		}
	}
	slices.SortFunc(summaries, func(first, second browserprotocol.ItemSummary) int {
		return cmp.Or(strings.Compare(strings.ToLower(first.Title), strings.ToLower(second.Title)), strings.Compare(first.ItemID, second.ItemID))
	})
	return summaries, nil
}

func (s *Service) summarizeBrowserItem(vaultID, vaultName string, vaultItem *item.Item) browserprotocol.ItemSummary {
	summary := browserprotocol.ItemSummary{
		VaultID: vaultID, ItemID: vaultItem.ID, Category: vaultItem.Category(), Title: vaultItem.Overview.Title,
		Subtitle: s.getBrowserItemSubtitle(vaultItem), VaultName: vaultName, IsFavorite: s.favorites.IsFavorite(vaultItem.ID),
		HasOneTimePassword: vaultItem.Details.OneTimePassword != "",
	}
	if len(vaultItem.Overview.URLs) > 0 {
		summary.URL = vaultItem.Overview.URLs[0]
	}
	return summary
}

// getBrowserItemSubtitle is the line under an item's title, as the app's lists show it; never a secret.
func (s *Service) getBrowserItemSubtitle(vaultItem *item.Item) string {
	switch {
	case vaultItem.IsCategory(item.CategoryCreditCard):
		card, _ := item.ReadCard(vaultItem)
		description := item.DescribeCard(card.Number, card.Expiry, s.config.Now())
		if description.Last4 == "" {
			return description.Brand
		}
		return strings.TrimSpace(description.Brand + " •••• " + description.Last4)
	case vaultItem.IsCategory(item.CategoryIdentity):
		identity, _ := item.ReadIdentity(vaultItem)
		if name := strings.Join(strings.Fields(identity.FirstName+" "+identity.LastName), " "); name != "" {
			return name
		}
		return identity.Email
	case vaultItem.IsCategory(item.CategoryAddress):
		address, _ := item.ReadAddress(vaultItem)
		return strings.Trim(address.City+", "+address.Country, ", ")
	case vaultItem.IsCategory(item.CategoryAPICredential):
		credential, _ := item.ReadAPICredential(vaultItem)
		return credential.Service
	case vaultItem.IsCategory(item.CategoryServer):
		server, _ := item.ReadServer(vaultItem)
		return server.Host
	case vaultItem.IsCategory(item.CategoryDatabase):
		database, _ := item.ReadDatabase(vaultItem)
		return database.Host
	case vaultItem.IsCategory(item.CategorySoftwareLicense):
		license, _ := item.ReadSoftwareLicense(vaultItem)
		return strings.TrimSpace(license.Product + " " + license.Version)
	case vaultItem.IsCategory(item.CategorySSHKey):
		return vaultItem.Overview.Fingerprint
	case vaultItem.IsCategory(item.CategorySecureNote):
		return ""
	}
	return vaultItem.Overview.Username
}

// findBrowserItem returns an item of a loaded vault, and the vault's name.
func (s *Service) findBrowserItem(ctx context.Context, vaultID, itemID string) (*item.Item, string, error) {
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, "", err
	}
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		return nil, "", err
	}
	vault := s.vaultsByID[vaultID]
	if vault == nil {
		return nil, "", ErrNotFound
	}
	vaultItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return nil, "", err
	}
	return vaultItem, vault.Name, nil
}

// getBrowserItem returns an item's fields for the popup, with the values of secrets left out.
func (s *Service) getBrowserItem(ctx context.Context, vaultID, itemID string) (browserprotocol.ItemResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	vaultItem, vaultName, err := s.findBrowserItem(ctx, vaultID, itemID)
	if err != nil {
		return browserprotocol.ItemResult{}, err
	}
	s.loadFavoritesOnce(ctx)
	fields := listBrowserItemFields(vaultItem)
	for index := range fields {
		if fields[index].Kind == browserprotocol.FieldKindSecret || fields[index].Kind == browserprotocol.FieldKindCode {
			fields[index].Value = ""
		}
	}
	return browserprotocol.ItemResult{ItemSummary: s.summarizeBrowserItem(vaultID, vaultName, vaultItem), Fields: fields}, nil
}

// revealBrowserItemField returns one field's value, a secret's too; a one-time password gives its current code, never
// the secret.
func (s *Service) revealBrowserItemField(ctx context.Context, vaultID, itemID, fieldID string) (browserprotocol.RevealResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	vaultItem, _, err := s.findBrowserItem(ctx, vaultID, itemID)
	if err != nil {
		return browserprotocol.RevealResult{}, err
	}
	for _, field := range listBrowserItemFields(vaultItem) {
		if field.ID != fieldID {
			continue
		}
		if field.Kind == browserprotocol.FieldKindCode {
			code, err := generateOneTimeCode(vaultItem, s.config.Now())
			return browserprotocol.RevealResult{Value: code.Code}, err
		}
		return browserprotocol.RevealResult{Value: field.Value}, nil
	}
	return browserprotocol.RevealResult{}, ErrNotFound
}

// getBrowserItemCode returns the current one-time code of any item that has one, for the popup's item view.
func (s *Service) getBrowserItemCode(ctx context.Context, vaultID, itemID string) (OneTimeCode, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	vaultItem, _, err := s.findBrowserItem(ctx, vaultID, itemID)
	if err != nil {
		return OneTimeCode{}, err
	}
	return generateOneTimeCode(vaultItem, s.config.Now())
}

// listBrowserItemFields lists what an item holds, in the order the app shows it, with every value: getBrowserItem
// removes the secrets'. The private key of an SSH key and passkeys are never listed. A field's ID stays the same while
// the item does not change, so items.reveal finds it.
func listBrowserItemFields(vaultItem *item.Item) []browserprotocol.ItemField {
	fields := []browserprotocol.ItemField{}
	add := func(id, label, kind, value string) {
		if value != "" || kind == browserprotocol.FieldKindCode {
			fields = append(fields, browserprotocol.ItemField{ID: id, Label: label, Kind: kind, Value: value})
		}
	}
	add("username", "Username", browserprotocol.FieldKindText, vaultItem.Overview.Username)
	add("password", "Password", browserprotocol.FieldKindSecret, vaultItem.Details.Password)
	if vaultItem.Details.OneTimePassword != "" {
		add("one-time-code", "One-time code", browserprotocol.FieldKindCode, "")
	}
	for index, itemURL := range vaultItem.Overview.URLs {
		label := "Website"
		if len(vaultItem.Overview.URLs) > 1 {
			label = "Website " + strconv.Itoa(index+1)
		}
		add("url:"+strconv.Itoa(index), label, browserprotocol.FieldKindURL, itemURL)
	}
	if vaultItem.IsCategory(item.CategorySSHKey) {
		add("public-key", "Public key", browserprotocol.FieldKindText, vaultItem.Overview.PublicKey)
		add("fingerprint", "Fingerprint", browserprotocol.FieldKindText, vaultItem.Overview.Fingerprint)
	}
	for index, field := range vaultItem.Details.Fields {
		kind := browserprotocol.FieldKindText
		if field.Concealed {
			kind = browserprotocol.FieldKindSecret
		}
		add("field:"+strconv.Itoa(index), field.Name, kind, field.Value)
	}
	add("notes", "Notes", browserprotocol.FieldKindNotes, vaultItem.Details.Notes)
	return fields
}

// setBrowserItemFavorite stars or unstars an item from the popup.
func (s *Service) setBrowserItemFavorite(vaultID, itemID string, isFavorite *bool) error {
	if isFavorite == nil {
		return browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "isFavorite is missing")
	}
	return s.SetItemFavorite(vaultID, itemID, *isFavorite)
}

// listWritableBrowserVaults lists the vaults a new item may go into: the personal vault first, then the others the user
// may write to, by name.
func (s *Service) listWritableBrowserVaults(ctx context.Context) ([]browserprotocol.VaultSummary, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, err
	}
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		return nil, err
	}
	vaults := []browserprotocol.VaultSummary{}
	for vaultID, vault := range s.vaultsByID {
		if apitypes.CanWriteItems(vault.Role) {
			vaults = append(vaults, browserprotocol.VaultSummary{VaultID: vaultID, Name: vault.Name, IsPersonal: vault.Type == apitypes.VaultPersonal})
		}
	}
	slices.SortFunc(vaults, func(first, second browserprotocol.VaultSummary) int {
		if first.IsPersonal != second.IsPersonal {
			if first.IsPersonal {
				return -1
			}
			return 1
		}
		return cmp.Or(strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name)), strings.Compare(first.VaultID, second.VaultID))
	})
	return vaults, nil
}

// requireWritableBrowserVault refuses a vault the user may only read, or does not have.
func (s *Service) requireWritableBrowserVault(ctx context.Context, vaultID string) error {
	vaults, err := s.listWritableBrowserVaults(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(vaults, func(vault browserprotocol.VaultSummary) bool { return vault.VaultID == vaultID }) {
		return browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "You cannot add items to that vault")
	}
	return nil
}

// createBrowserItem saves a new login, secure note, credit card or identity from the popup into a vault the user may
// write to.
func (s *Service) createBrowserItem(ctx context.Context, vaultID string, newItem *browserprotocol.NewItem) (browserprotocol.ItemSummary, error) {
	if newItem == nil {
		return browserprotocol.ItemSummary{}, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "the item is missing")
	}
	if err := s.requireWritableBrowserVault(ctx, vaultID); err != nil {
		return browserprotocol.ItemSummary{}, err
	}
	detail := ItemDetail{Category: newItem.Category, Title: strings.TrimSpace(newItem.Title), Notes: newItem.Notes, URLs: []string{}, Tags: []string{}, Fields: []ItemField{}}
	switch newItem.Category {
	case item.CategoryLogin:
		detail.Username, detail.Password = newItem.Username, newItem.Password
		if itemURL := strings.TrimSpace(newItem.URL); itemURL != "" {
			detail.URLs = []string{itemURL}
		}
	case item.CategorySecureNote:
	case item.CategoryCreditCard:
		detail.Card = &ItemCard{Cardholder: newItem.Cardholder, Number: newItem.CardNumber, Expiry: newItem.Expiry, SecurityCode: newItem.SecurityCode}
	case item.CategoryIdentity:
		detail.Identity = &ItemIdentity{FirstName: newItem.FirstName, LastName: newItem.LastName, Email: newItem.Email, Phone: newItem.Phone}
	default:
		return browserprotocol.ItemSummary{}, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "Hisar cannot create that kind of item from the browser")
	}
	saved, err := s.SaveItem(vaultID, detail)
	if err != nil {
		var responseError *browserprotocol.ResponseError
		if errors.As(err, &responseError) || errors.Is(err, ErrLocked) || errors.Is(err, ErrSignedOut) || errors.Is(err, ErrOffline) {
			return browserprotocol.ItemSummary{}, err
		}
		// SaveItem's refusals, such as a missing title, are sentences for the user.
		return browserprotocol.ItemSummary{}, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, err.Error())
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	vaultItem, err := s.findItem(vaultID, saved.ID)
	if err != nil {
		return browserprotocol.ItemSummary{}, err
	}
	return s.summarizeBrowserItem(vaultID, s.vaultsByID[vaultID].Name, vaultItem), nil
}

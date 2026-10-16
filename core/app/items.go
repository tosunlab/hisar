package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/totp"
)

// ListItems fetches the vault's items from the server and returns them without secrets in a stable order.
func (s *Service) ListItems(vaultID string) ([]ItemDetail, error) {
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
	items, err := s.unlockedSession.ListItems(ctx, vault)
	if err != nil {
		return nil, convertError(err)
	}
	s.itemsByVaultID[vaultID] = items
	s.loadFavoritesOnce(ctx)
	summaries := make([]ItemDetail, 0, len(items))
	for index := range items {
		summaries = append(summaries, s.summarizeItem(&items[index]))
	}
	return summaries, nil
}

// summarizeItem is what lists show: no password, notes, fields or one-time password secret.
func (s *Service) summarizeItem(vaultItem *item.Item) ItemDetail {
	summary := s.describeItem(vaultItem)
	summary.Password, summary.Notes, summary.Fields, summary.OneTimePassword = "", "", nil, ""
	if summary.Card != nil {
		summary.Card = &ItemCard{Type: summary.Card.Type, CardDescription: CardDescription{Brand: summary.Card.Brand, Last4: summary.Card.Last4}}
	}
	if identity := summary.Identity; identity != nil {
		// Lists show the name; search also finds an identity by its address, where imports put a person's address.
		summary.Identity = &ItemIdentity{
			FirstName: identity.FirstName, MiddleName: identity.MiddleName, LastName: identity.LastName, Email: identity.Email,
			ItemAddress: summarizeAddress(identity.ItemAddress),
		}
	}
	if credential := summary.APICredential; credential != nil {
		// The key ID is not shown in lists, but search finds a credential by it.
		summary.APICredential = &ItemAPICredential{Service: credential.Service, KeyID: credential.KeyID, Host: credential.Host}
	}
	if address := summary.Address; address != nil {
		// The first line of the street is a list's subtitle when the address has no city or country.
		addressSummary := summarizeAddress(*address)
		summary.Address = &addressSummary
	}
	if license := summary.SoftwareLicense; license != nil {
		// Lists show the version and licensed-to name; search also finds a license by its product and email.
		summary.SoftwareLicense = &ItemSoftwareLicense{Product: license.Product, Version: license.Version, LicensedTo: license.LicensedTo, Email: license.Email}
	}
	if server := summary.Server; server != nil {
		// Lists show the host and port; the linked key is looked up only in the item.
		summary.Server = &ItemServer{Host: server.Host, Port: server.Port}
	}
	return summary
}

// summarizeAddress keeps what lists show and search reads: the first line of the street, the city and the country.
func summarizeAddress(address ItemAddress) ItemAddress {
	firstLine, _, _ := strings.Cut(address.Street, "\n")
	return ItemAddress{Street: firstLine, City: address.City, Country: address.Country}
}

func (s *Service) describeItem(vaultItem *item.Item) ItemDetail {
	detail := convertToItemDetail(vaultItem, s.config.Now())
	detail.IsFavorite = s.favorites.IsFavorite(vaultItem.ID)
	if server := detail.Server; server != nil && server.SSHKeyID != "" {
		// The linked key is in the server's vault; item IDs are unique, so any loaded vault will do.
		for _, items := range s.itemsByVaultID {
			for index := range items {
				if key := &items[index]; key.ID == server.SSHKeyID && key.IsCategory(item.CategorySSHKey) {
					server.SSHKeyTitle, server.SSHKeyFingerprint = key.Overview.Title, key.Overview.Fingerprint
				}
			}
		}
	}
	return detail
}

func (s *Service) findItem(vaultID, itemID string) (*item.Item, error) {
	for index, vaultItem := range s.itemsByVaultID[vaultID] {
		if vaultItem.ID == itemID {
			return &s.itemsByVaultID[vaultID][index], nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.loadFavoritesOnce(ctx)
	return s.describeItem(vaultItem), nil
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

// convertToItemDetail checks a card's expiry at now.
func convertToItemDetail(vaultItem *item.Item, now time.Time) ItemDetail {
	detail := ItemDetail{
		ID:          vaultItem.ID,
		Revision:    vaultItem.Revision,
		Category:    vaultItem.Overview.Category,
		Title:       vaultItem.Overview.Title,
		Username:    vaultItem.Overview.Username,
		URLs:        append([]string{}, vaultItem.Overview.URLs...),
		Tags:        append([]string{}, vaultItem.Overview.Tags...),
		Password:    vaultItem.Details.Password,
		Notes:       vaultItem.Details.Notes,
		Fields:      []ItemField{},
		PublicKey:   vaultItem.Overview.PublicKey,
		Fingerprint: vaultItem.Overview.Fingerprint,
		KeyType:     vaultItem.Overview.KeyType,

		OneTimePassword:    vaultItem.Details.OneTimePassword,
		HasOneTimePassword: vaultItem.Details.OneTimePassword != "",

		Passkeys: convertToItemPasskeys(vaultItem.Details.Passkeys),

		CanConvertToSecureNote: vaultItem.IsCategory(item.CategoryLogin) && item.HasOnlyNotes(vaultItem),
	}
	fields := vaultItem.Details.Fields
	if vaultItem.IsCategory(item.CategoryCreditCard) {
		var card item.Card
		card, fields = item.ReadCard(vaultItem)
		detail.Card = &ItemCard{
			Cardholder: card.Cardholder, Type: card.Type, Number: card.Number, Expiry: card.Expiry, SecurityCode: card.SecurityCode, PIN: card.PIN,
			ItemAddress: convertToItemAddress(card.BillingAddress), BillingAddress: card.BillingAddressText,
			CardDescription: convertToCardDescription(item.DescribeCard(card.Number, card.Expiry, now)),
		}
	}
	if vaultItem.IsCategory(item.CategoryIdentity) {
		var identity item.Identity
		identity, fields = item.ReadIdentity(vaultItem)
		detail.Identity = convertToItemIdentity(identity)
	}
	if vaultItem.IsCategory(item.CategoryAPICredential) {
		var credential item.APICredential
		credential, fields = item.ReadAPICredential(vaultItem)
		detail.APICredential = &ItemAPICredential{
			Service: credential.Service, KeyID: credential.KeyID, Secret: credential.Secret, Host: credential.Host, Expiry: credential.Expiry,
			ExpiryWarning: item.DescribeAPICredentialExpiry(credential.Expiry, now),
		}
	}
	if vaultItem.IsCategory(item.CategoryAddress) {
		var address item.Address
		address, fields = item.ReadAddress(vaultItem)
		itemAddress := convertToItemAddress(address)
		detail.Address = &itemAddress
	}
	if vaultItem.IsCategory(item.CategorySoftwareLicense) {
		var license item.SoftwareLicense
		license, fields = item.ReadSoftwareLicense(vaultItem)
		detail.SoftwareLicense = convertToItemSoftwareLicense(license)
	}
	if vaultItem.IsCategory(item.CategoryServer) {
		var server item.Server
		server, fields = item.ReadServer(vaultItem)
		detail.Server = &ItemServer{Host: server.Host, Port: server.Port, SSHKeyID: server.SSHKeyID, PortWarning: server.PortWarning()}
	}
	for _, field := range fields {
		detail.Fields = append(detail.Fields, ItemField{Name: field.Name, Value: field.Value, Concealed: field.Concealed})
	}
	return detail
}

func convertToCardDescription(description item.CardDescription) CardDescription {
	return CardDescription{Brand: description.Brand, Last4: description.Last4, NumberWarning: description.NumberWarning, ExpiryWarning: description.ExpiryWarning}
}

// DescribeCard names the brand and last four digits of a card number and warns about a number or expiry that looks
// wrong, for an editor to show while the user types.
func (s *Service) DescribeCard(number, expiry string) CardDescription {
	return convertToCardDescription(item.DescribeCard(number, expiry, s.config.Now()))
}

// getStoredFields returns an item's custom fields as they are stored: a card's, identity's, API credential's, address's or
// software license's values first, as named fields.
func getStoredFields(detail ItemDetail) []ItemField {
	var vaultItem item.Item
	switch {
	case detail.Card != nil:
		item.WriteCard(&vaultItem, convertFromItemCard(detail.Card), nil)
	case detail.Identity != nil:
		item.WriteIdentity(&vaultItem, convertFromItemIdentity(detail.Identity), nil)
	case detail.APICredential != nil:
		item.WriteAPICredential(&vaultItem, convertFromItemAPICredential(detail.APICredential), nil)
	case detail.Address != nil:
		item.WriteAddress(&vaultItem, convertFromItemAddress(*detail.Address), nil)
	case detail.SoftwareLicense != nil:
		item.WriteSoftwareLicense(&vaultItem, convertFromItemSoftwareLicense(detail.SoftwareLicense), nil)
	case detail.Server != nil:
		item.WriteServer(&vaultItem, convertFromItemServer(detail.Server), nil)
	default:
		return detail.Fields
	}
	fields := []ItemField{}
	for _, field := range vaultItem.Details.Fields {
		fields = append(fields, ItemField{Name: field.Name, Value: field.Value, Concealed: field.Concealed})
	}
	return append(fields, detail.Fields...)
}

func convertFromItemCard(card *ItemCard) item.Card {
	return item.Card{
		Cardholder: card.Cardholder, Type: card.Type, Number: card.Number, Expiry: card.Expiry, SecurityCode: card.SecurityCode, PIN: card.PIN,
		BillingAddress: convertFromItemAddress(card.ItemAddress), BillingAddressText: card.BillingAddress,
	}
}

func convertToItemAddress(address item.Address) ItemAddress {
	return ItemAddress{Street: address.Street, City: address.City, Region: address.Region, PostalCode: address.PostalCode, Country: address.Country}
}

func convertFromItemAddress(address ItemAddress) item.Address {
	return item.Address{Street: address.Street, City: address.City, Region: address.Region, PostalCode: address.PostalCode, Country: address.Country}
}

func convertToItemIdentity(identity item.Identity) *ItemIdentity {
	return &ItemIdentity{
		FirstName: identity.FirstName, MiddleName: identity.MiddleName, LastName: identity.LastName, BirthDate: identity.BirthDate,
		Email: identity.Email, Phone: identity.Phone, ItemAddress: convertToItemAddress(identity.Address), Address: identity.AddressText,
		PassportNumber: identity.PassportNumber, DriversLicenseNumber: identity.DriversLicenseNumber, NationalIDNumber: identity.NationalIDNumber,
	}
}

func convertFromItemIdentity(identity *ItemIdentity) item.Identity {
	return item.Identity{
		FirstName: identity.FirstName, MiddleName: identity.MiddleName, LastName: identity.LastName, BirthDate: identity.BirthDate,
		Email: identity.Email, Phone: identity.Phone, Address: convertFromItemAddress(identity.ItemAddress), AddressText: identity.Address,
		PassportNumber: identity.PassportNumber, DriversLicenseNumber: identity.DriversLicenseNumber, NationalIDNumber: identity.NationalIDNumber,
	}
}

func convertFromItemServer(server *ItemServer) item.Server {
	return item.Server{Host: server.Host, Port: server.Port, SSHKeyID: server.SSHKeyID}
}

func convertToItemSoftwareLicense(license item.SoftwareLicense) *ItemSoftwareLicense {
	return &ItemSoftwareLicense{
		Product: license.Product, Version: license.Version, LicenseKey: license.LicenseKey, LicensedTo: license.LicensedTo, Email: license.Email,
		PurchaseDate: license.PurchaseDate, DownloadURL: license.DownloadURL,
	}
}

func convertFromItemSoftwareLicense(license *ItemSoftwareLicense) item.SoftwareLicense {
	return item.SoftwareLicense{
		Product: license.Product, Version: license.Version, LicenseKey: license.LicenseKey, LicensedTo: license.LicensedTo, Email: license.Email,
		PurchaseDate: license.PurchaseDate, DownloadURL: license.DownloadURL,
	}
}

func convertFromItemAPICredential(credential *ItemAPICredential) item.APICredential {
	return item.APICredential{Service: credential.Service, KeyID: credential.KeyID, Secret: credential.Secret, Host: credential.Host, Expiry: credential.Expiry}
}

func convertFromItemDetail(detail ItemDetail) item.Item {
	vaultItem := item.Item{
		ID:       detail.ID,
		Revision: detail.Revision,
		Overview: item.Overview{Title: strings.TrimSpace(detail.Title), Username: strings.TrimSpace(detail.Username), Tags: item.NormalizeTags(detail.Tags)},
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
	if err := s.checkServerSSHKey(ctx, vault, &vaultItem); err != nil {
		return ItemDetail{}, err
	}
	if _, err := s.unlockedSession.SaveItem(ctx, vault, &vaultItem); err != nil {
		return ItemDetail{}, convertError(err)
	}
	s.replaceCachedItem(vaultID, vaultItem)
	return s.describeItem(&vaultItem), nil
}

// buildSavedItem keeps the category and passkeys of an existing item; an ssh-key item only takes the title, tags and notes
// from the editor. A secure note or a card keeps no login data, and a card's values become its named fields. Login data an
// existing one already has, which its editor does not show, is taken from the stored item, not from detail.
func (s *Service) buildSavedItem(vaultID string, detail ItemDetail) (item.Item, error) {
	var cachedItem *item.Item
	if detail.ID != "" {
		cachedItem, _ = s.findItem(vaultID, detail.ID)
	}
	if cachedItem == nil {
		switch detail.Category {
		case "", item.CategoryLogin, item.CategorySecureNote, item.CategoryCreditCard, item.CategoryIdentity, item.CategoryAPICredential, item.CategoryAddress,
			item.CategorySoftwareLicense, item.CategoryServer:
		case item.CategorySSHKey:
			if detail.ID == "" {
				return item.Item{}, errors.New("generate or import an SSH key instead")
			}
			return item.Item{}, errors.New("this SSH key is not loaded; reload the vault and try again")
		default:
			return item.Item{}, fmt.Errorf("unknown item type %q", detail.Category)
		}
		vaultItem := convertFromItemDetail(detail)
		vaultItem.Overview.Category = detail.Category
		applyCategory(&vaultItem, detail)
		return vaultItem, nil
	}
	if !cachedItem.IsCategory(item.CategorySSHKey) {
		vaultItem := convertFromItemDetail(detail)
		vaultItem.Overview.Category = cachedItem.Overview.Category
		vaultItem.Details.Passkeys = cachedItem.Details.Passkeys
		item.UpdatePasskeyOverview(&vaultItem)
		if cachedItem.OmitsLoginData() {
			vaultItem.Overview.Username, vaultItem.Overview.URLs = cachedItem.Overview.Username, cachedItem.Overview.URLs
			vaultItem.Details.Password, vaultItem.Details.OneTimePassword = cachedItem.Details.Password, cachedItem.Details.OneTimePassword
		}
		applyCategory(&vaultItem, detail)
		return vaultItem, nil
	}
	vaultItem := *cachedItem
	vaultItem.Revision = detail.Revision
	vaultItem.Overview.Title = strings.TrimSpace(detail.Title)
	vaultItem.Overview.Tags = item.NormalizeTags(detail.Tags)
	vaultItem.Details.Notes = detail.Notes
	vaultItem.Details.OneTimePassword = detail.OneTimePassword
	return vaultItem, nil
}

// applyCategory shapes a saved item to its category: a secure note, card, identity or API credential moves login data into
// custom fields, and a card's, identity's or API credential's values from detail become its named fields. A category from
// a newer app keeps everything, since the editors show it as a login. Without detail.Card, detail.Identity or
// detail.APICredential (the CLI), the named fields come in detail.Fields.
func applyCategory(vaultItem *item.Item, detail ItemDetail) {
	switch {
	case vaultItem.IsCategory(item.CategoryCreditCard) && detail.Card != nil:
		item.WriteCard(vaultItem, convertFromItemCard(detail.Card), vaultItem.Details.Fields)
	case vaultItem.IsCategory(item.CategoryIdentity) && detail.Identity != nil:
		item.WriteIdentity(vaultItem, convertFromItemIdentity(detail.Identity), vaultItem.Details.Fields)
	case vaultItem.IsCategory(item.CategoryAPICredential) && detail.APICredential != nil:
		item.WriteAPICredential(vaultItem, convertFromItemAPICredential(detail.APICredential), vaultItem.Details.Fields)
	case vaultItem.IsCategory(item.CategoryAddress) && detail.Address != nil:
		item.WriteAddress(vaultItem, convertFromItemAddress(*detail.Address), vaultItem.Details.Fields)
	case vaultItem.IsCategory(item.CategorySoftwareLicense) && detail.SoftwareLicense != nil:
		item.WriteSoftwareLicense(vaultItem, convertFromItemSoftwareLicense(detail.SoftwareLicense), vaultItem.Details.Fields)
	case vaultItem.IsCategory(item.CategoryServer) && detail.Server != nil:
		item.WriteServer(vaultItem, convertFromItemServer(detail.Server), vaultItem.Details.Fields)
	}
	if vaultItem.OmitsLoginData() {
		vaultItem.MoveLoginDataToFields()
	}
	item.NormalizeCard(vaultItem)
	item.NormalizeIdentity(vaultItem)
	item.NormalizeAPICredential(vaultItem)
	item.NormalizeAddress(vaultItem)
	item.NormalizeSoftwareLicense(vaultItem)
	item.NormalizeServer(vaultItem)
}

// checkServerSSHKey refuses a server linked to anything but an SSH key of its own vault, so everyone who sees the server
// sees the key. A link the stored item already has is kept even when its key was deleted, so the server stays editable;
// the apps show it as missing. It needs s.mutex.
func (s *Service) checkServerSSHKey(ctx context.Context, vault *session.Vault, vaultItem *item.Item) error {
	if !vaultItem.IsCategory(item.CategoryServer) {
		return nil
	}
	server, _ := item.ReadServer(vaultItem)
	if server.SSHKeyID == "" {
		return nil
	}
	if vaultItem.ID != "" {
		if stored, err := s.findItem(vault.ID, vaultItem.ID); err == nil {
			if storedServer, _ := item.ReadServer(stored); storedServer.SSHKeyID == server.SSHKeyID {
				return nil
			}
		}
	}
	if _, isLoaded := s.itemsByVaultID[vault.ID]; !isLoaded {
		items, err := s.unlockedSession.ListItems(ctx, vault)
		if err != nil {
			return convertError(err)
		}
		s.itemsByVaultID[vault.ID] = items
	}
	if key, err := s.findItem(vault.ID, server.SSHKeyID); err != nil || !key.IsCategory(item.CategorySSHKey) {
		return errors.New("choose an SSH key from this vault")
	}
	return nil
}

// ConvertToSecureNote turns a login with only notes and custom fields, such as one imported before Hisar had secure notes,
// into a secure note.
func (s *Service) ConvertToSecureNote(vaultID, itemID string) (ItemDetail, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ItemDetail{}, err
	}
	cachedItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return ItemDetail{}, err
	}
	if !cachedItem.IsCategory(item.CategoryLogin) || !item.HasOnlyNotes(cachedItem) {
		return ItemDetail{}, errors.New("only a login with nothing but notes and custom fields can become a secure note")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return ItemDetail{}, err
	}
	vaultItem := *cachedItem
	vaultItem.Overview.Category = item.CategorySecureNote
	if _, err := s.unlockedSession.SaveItem(ctx, vault, &vaultItem); err != nil {
		return ItemDetail{}, convertError(err)
	}
	s.replaceCachedItem(vaultID, vaultItem)
	return s.describeItem(&vaultItem), nil
}

func (s *Service) replaceCachedItem(vaultID string, vaultItem item.Item) {
	items := s.itemsByVaultID[vaultID]
	for index := range items {
		if items[index].ID == vaultItem.ID {
			items[index] = vaultItem
			return
		}
	}
	s.itemsByVaultID[vaultID] = append(items, vaultItem)
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
	remaining := s.itemsByVaultID[vaultID][:0]
	for _, cachedItem := range s.itemsByVaultID[vaultID] {
		if cachedItem.ID != itemID {
			remaining = append(remaining, cachedItem)
		}
	}
	s.itemsByVaultID[vaultID] = remaining
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

package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
)

var errNoURLMatch = errors.New("this login is not saved for this site")

// listLoginsForURL loads every vault's items if needed and returns the logins whose URLs match pageURL.
func (s *Service) listLoginsForURL(ctx context.Context, pageURL string) ([]browserprotocol.Login, map[string]item.Item, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, nil, err
	}
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		return nil, nil, err
	}
	logins := []browserprotocol.Login{}
	itemsByKey := map[string]item.Item{}
	for vaultID, items := range s.itemsByVaultID {
		vault := s.vaultsByID[vaultID]
		if vault == nil {
			continue
		}
		for _, vaultItem := range items {
			if !vaultItem.OffersLoginFill() || !slices.ContainsFunc(vaultItem.Overview.URLs, func(itemURL string) bool {
				return browserprotocol.IsURLMatch(itemURL, pageURL, s.state.IsExactHostMatch)
			}) {
				continue
			}
			logins = append(logins, browserprotocol.Login{
				VaultID: vaultID, ItemID: vaultItem.ID, Title: vaultItem.Overview.Title, Username: vaultItem.Overview.Username, VaultName: vault.Name,
				HasOneTimePassword: vaultItem.Details.OneTimePassword != "",
			})
			itemsByKey[vaultID+"/"+vaultItem.ID] = vaultItem
		}
	}
	slices.SortFunc(logins, func(first, second browserprotocol.Login) int {
		if comparison := strings.Compare(strings.ToLower(first.Title), strings.ToLower(second.Title)); comparison != 0 {
			return comparison
		}
		return strings.Compare(strings.ToLower(first.Username), strings.ToLower(second.Username))
	})
	return logins, itemsByKey, nil
}

// requireUnlockedWithoutActivity checks the lock state like requireUnlocked but, like the SSH agent, does not postpone auto-lock.
func (s *Service) requireUnlockedWithoutActivity() error {
	if s.unlockedSession != nil {
		return nil
	}
	if s.state.IsLoggedIn() {
		return ErrLocked
	}
	return ErrSignedOut
}

func (s *Service) loadItemsOfAllVault(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if s.vaultsByID == nil {
		if err := s.loadVaults(ctx); err != nil {
			return err
		}
	}
	for vaultID, vault := range s.vaultsByID {
		if _, isLoaded := s.itemsByVaultID[vaultID]; isLoaded {
			continue
		}
		items, err := s.unlockedSession.ListItems(ctx, vault)
		if err != nil {
			return convertError(err)
		}
		s.itemsByVaultID[vaultID] = items
	}
	return nil
}

// ListLoginsForURL and GetLoginForURL serve the mobile AutoFill extensions; as functions, Wails does not bind them.
func ListLoginsForURL(s *Service, pageURL string) ([]browserprotocol.Login, error) {
	logins, err := s.listMatchingLogins(context.Background(), pageURL)
	return logins, convertBrowserError(err)
}

func GetLoginForURL(s *Service, vaultID, itemID, pageURL string) (browserprotocol.FillResult, error) {
	result, err := s.getLoginForFill(context.Background(), vaultID, itemID, pageURL)
	return result, convertBrowserError(err)
}

func GetOneTimeCodeForURL(s *Service, vaultID, itemID, pageURL string) (OneTimeCode, error) {
	result, err := s.getOneTimeCodeForFill(context.Background(), vaultID, itemID, pageURL)
	return result, convertBrowserError(err)
}

func (s *Service) listMatchingLogins(ctx context.Context, pageURL string) ([]browserprotocol.Login, error) {
	logins, _, err := s.listLoginsForURL(ctx, pageURL)
	return logins, err
}

func (s *Service) getLoginForFill(ctx context.Context, vaultID, itemID, pageURL string) (browserprotocol.FillResult, error) {
	_, itemsByKey, err := s.listLoginsForURL(ctx, pageURL)
	if err != nil {
		return browserprotocol.FillResult{}, err
	}
	vaultItem, found := itemsByKey[vaultID+"/"+itemID]
	if !found {
		return browserprotocol.FillResult{}, errNoURLMatch
	}
	return browserprotocol.FillResult{Username: vaultItem.Overview.Username, Password: vaultItem.Details.Password, HasOneTimePassword: vaultItem.Details.OneTimePassword != ""}, nil
}

// getOneTimeCodeForFill applies the fill rule: the item must match pageURL. Only the code leaves, never the secret.
func (s *Service) getOneTimeCodeForFill(ctx context.Context, vaultID, itemID, pageURL string) (OneTimeCode, error) {
	_, itemsByKey, err := s.listLoginsForURL(ctx, pageURL)
	if err != nil {
		return OneTimeCode{}, err
	}
	vaultItem, found := itemsByKey[vaultID+"/"+itemID]
	if !found {
		return OneTimeCode{}, errNoURLMatch
	}
	return generateOneTimeCode(&vaultItem, s.config.Now())
}

func (s *Service) checkLogin(ctx context.Context, pageURL, username, password string) (browserprotocol.CheckResult, error) {
	logins, itemsByKey, err := s.listLoginsForURL(ctx, pageURL)
	if err != nil {
		return browserprotocol.CheckResult{}, err
	}
	var candidates []browserprotocol.Login
	for _, login := range logins {
		if strings.EqualFold(strings.TrimSpace(login.Username), strings.TrimSpace(username)) {
			candidates = append(candidates, login)
		}
	}
	if len(candidates) == 0 && strings.TrimSpace(username) == "" && len(logins) == 1 {
		candidates = logins
	}
	if len(candidates) == 0 {
		return browserprotocol.CheckResult{Status: "new"}, nil
	}
	for _, candidate := range candidates {
		if itemsByKey[candidate.VaultID+"/"+candidate.ItemID].Details.Password == password {
			return browserprotocol.CheckResult{Status: "unchanged", VaultID: candidate.VaultID, ItemID: candidate.ItemID, Title: candidate.Title}, nil
		}
	}
	first := candidates[0]
	return browserprotocol.CheckResult{Status: "changed", VaultID: first.VaultID, ItemID: first.ItemID, Title: first.Title}, nil
}

func (s *Service) saveLogin(ctx context.Context, request *browserprotocol.Request) (browserprotocol.SaveResult, error) {
	if request.Password == "" {
		return browserprotocol.SaveResult{}, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "the password is empty")
	}
	if request.ItemID != "" {
		_, itemsByKey, err := s.listLoginsForURL(ctx, request.URL)
		if err != nil {
			return browserprotocol.SaveResult{}, err
		}
		vaultItem, found := itemsByKey[request.VaultID+"/"+request.ItemID]
		if !found {
			return browserprotocol.SaveResult{}, errNoURLMatch
		}
		detail := convertToItemDetail(&vaultItem, s.config.Now())
		detail.Username, detail.Password = request.Username, request.Password
		saved, err := s.SaveItem(request.VaultID, detail)
		if err != nil {
			return browserprotocol.SaveResult{}, err
		}
		return browserprotocol.SaveResult{VaultID: request.VaultID, ItemID: saved.ID}, nil
	}
	vaultID, err := s.findPersonalVaultID(ctx)
	if err != nil {
		return browserprotocol.SaveResult{}, err
	}
	saved, err := s.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryLogin, Title: browserprotocol.GetSiteName(request.URL), Username: request.Username, Password: request.Password,
		URLs: []string{browserprotocol.GetOrigin(request.URL)}, Fields: []ItemField{},
	})
	if err != nil {
		return browserprotocol.SaveResult{}, err
	}
	return browserprotocol.SaveResult{VaultID: vaultID, ItemID: saved.ID}, nil
}

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

// listAllLoginForURL loads every vault's items if needed and returns the logins whose URLs match pageURL.
func (s *Service) listAllLoginForURL(ctx context.Context, pageURL string) ([]browserprotocol.Login, map[string]item.Item, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, nil, err
	}
	if err := s.loadAllItemOfAllVault(ctx); err != nil {
		return nil, nil, err
	}
	allLogin := []browserprotocol.Login{}
	allItemByKey := map[string]item.Item{}
	for vaultID, allItem := range s.allItemByVaultID {
		vault := s.allVaultByID[vaultID]
		if vault == nil {
			continue
		}
		for _, vaultItem := range allItem {
			if !isLoginItem(&vaultItem) || !slices.ContainsFunc(vaultItem.Overview.URLs, func(itemURL string) bool {
				return browserprotocol.IsURLMatch(itemURL, pageURL, s.state.IsExactHostMatch)
			}) {
				continue
			}
			allLogin = append(allLogin, browserprotocol.Login{
				VaultID: vaultID, ItemID: vaultItem.ID, Title: vaultItem.Overview.Title, Username: vaultItem.Overview.Username, VaultName: vault.Name,
				HasOneTimePassword: vaultItem.Details.OneTimePassword != "",
			})
			allItemByKey[vaultID+"/"+vaultItem.ID] = vaultItem
		}
	}
	slices.SortFunc(allLogin, func(first, second browserprotocol.Login) int {
		if comparison := strings.Compare(strings.ToLower(first.Title), strings.ToLower(second.Title)); comparison != 0 {
			return comparison
		}
		return strings.Compare(strings.ToLower(first.Username), strings.ToLower(second.Username))
	})
	return allLogin, allItemByKey, nil
}

func isLoginItem(vaultItem *item.Item) bool {
	return vaultItem.Overview.Category == "" || vaultItem.Overview.Category == item.CategoryLogin
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

func (s *Service) loadAllItemOfAllVault(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if s.allVaultByID == nil {
		if err := s.loadAllVault(ctx); err != nil {
			return err
		}
	}
	for vaultID, vault := range s.allVaultByID {
		if _, isLoaded := s.allItemByVaultID[vaultID]; isLoaded {
			continue
		}
		allItem, err := s.unlockedSession.ListAllItem(ctx, vault)
		if err != nil {
			return convertError(err)
		}
		s.allItemByVaultID[vaultID] = allItem
	}
	return nil
}

// ListAllLoginForURL and GetLoginForURL serve the mobile AutoFill extensions; as functions, Wails does not bind them.
func ListAllLoginForURL(s *Service, pageURL string) ([]browserprotocol.Login, error) {
	allLogin, err := s.listAllMatchingLogin(context.Background(), pageURL)
	return allLogin, convertBrowserError(err)
}

func GetLoginForURL(s *Service, vaultID, itemID, pageURL string) (browserprotocol.FillResult, error) {
	result, err := s.getLoginForFill(context.Background(), vaultID, itemID, pageURL)
	return result, convertBrowserError(err)
}

func GetOneTimeCodeForURL(s *Service, vaultID, itemID, pageURL string) (OneTimeCode, error) {
	result, err := s.getOneTimeCodeForFill(context.Background(), vaultID, itemID, pageURL)
	return result, convertBrowserError(err)
}

func (s *Service) listAllMatchingLogin(ctx context.Context, pageURL string) ([]browserprotocol.Login, error) {
	allLogin, _, err := s.listAllLoginForURL(ctx, pageURL)
	return allLogin, err
}

func (s *Service) getLoginForFill(ctx context.Context, vaultID, itemID, pageURL string) (browserprotocol.FillResult, error) {
	_, allItemByKey, err := s.listAllLoginForURL(ctx, pageURL)
	if err != nil {
		return browserprotocol.FillResult{}, err
	}
	vaultItem, found := allItemByKey[vaultID+"/"+itemID]
	if !found {
		return browserprotocol.FillResult{}, errNoURLMatch
	}
	return browserprotocol.FillResult{Username: vaultItem.Overview.Username, Password: vaultItem.Details.Password, HasOneTimePassword: vaultItem.Details.OneTimePassword != ""}, nil
}

// getOneTimeCodeForFill applies the fill rule: the item must match pageURL. Only the code leaves, never the secret.
func (s *Service) getOneTimeCodeForFill(ctx context.Context, vaultID, itemID, pageURL string) (OneTimeCode, error) {
	_, allItemByKey, err := s.listAllLoginForURL(ctx, pageURL)
	if err != nil {
		return OneTimeCode{}, err
	}
	vaultItem, found := allItemByKey[vaultID+"/"+itemID]
	if !found {
		return OneTimeCode{}, errNoURLMatch
	}
	return generateOneTimeCode(&vaultItem, s.config.Now())
}

func (s *Service) checkLogin(ctx context.Context, pageURL, username, password string) (browserprotocol.CheckResult, error) {
	allLogin, allItemByKey, err := s.listAllLoginForURL(ctx, pageURL)
	if err != nil {
		return browserprotocol.CheckResult{}, err
	}
	var allCandidate []browserprotocol.Login
	for _, login := range allLogin {
		if strings.EqualFold(strings.TrimSpace(login.Username), strings.TrimSpace(username)) {
			allCandidate = append(allCandidate, login)
		}
	}
	if len(allCandidate) == 0 && strings.TrimSpace(username) == "" && len(allLogin) == 1 {
		allCandidate = allLogin
	}
	if len(allCandidate) == 0 {
		return browserprotocol.CheckResult{Status: "new"}, nil
	}
	for _, candidate := range allCandidate {
		if allItemByKey[candidate.VaultID+"/"+candidate.ItemID].Details.Password == password {
			return browserprotocol.CheckResult{Status: "unchanged", VaultID: candidate.VaultID, ItemID: candidate.ItemID, Title: candidate.Title}, nil
		}
	}
	first := allCandidate[0]
	return browserprotocol.CheckResult{Status: "changed", VaultID: first.VaultID, ItemID: first.ItemID, Title: first.Title}, nil
}

func (s *Service) saveLogin(ctx context.Context, request *browserprotocol.Request) (browserprotocol.SaveResult, error) {
	if request.Password == "" {
		return browserprotocol.SaveResult{}, browserprotocol.CreateError(browserprotocol.ErrorInvalidRequest, "the password is empty")
	}
	if request.ItemID != "" {
		_, allItemByKey, err := s.listAllLoginForURL(ctx, request.URL)
		if err != nil {
			return browserprotocol.SaveResult{}, err
		}
		vaultItem, found := allItemByKey[request.VaultID+"/"+request.ItemID]
		if !found {
			return browserprotocol.SaveResult{}, errNoURLMatch
		}
		detail := convertToItemDetail(&vaultItem)
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

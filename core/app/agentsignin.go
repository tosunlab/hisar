package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/signin"
)

// AgentSignIn is Settings → Coding agents → Website sign-in. IsAvailable: this app can fill browser windows (Hisar for
// Mac and Linux). HasAccessibility: the system lets it (Mac: the Accessibility permission; Linux: an X11 display and the
// accessibility bus).
type AgentSignIn struct {
	IsAvailable      bool `json:"isAvailable"`
	IsEnabled        bool `json:"isEnabled"`
	HasAccessibility bool `json:"hasAccessibility"`
}

// GetAgentSignIn reads Settings → Coding agents → Website sign-in, and whether the system lets Hisar read and fill
// browser windows now.
func (s *Service) GetAgentSignIn() AgentSignIn {
	s.mutex.Lock()
	isEnabled := s.state.IsAgentSignInEnabled
	s.mutex.Unlock()
	driver := s.config.SignInDriver
	return AgentSignIn{IsAvailable: driver != nil, IsEnabled: isEnabled, HasAccessibility: driver != nil && driver.HasAccess()}
}

// SetAgentSignInEnabled turns website sign-in for agents on or off; turning it on without the Accessibility permission
// has the Mac ask for it.
func (s *Service) SetAgentSignInEnabled(isEnabled bool) (AgentSignIn, error) {
	driver := s.config.SignInDriver
	if driver == nil {
		return s.GetAgentSignIn(), errors.New("only Hisar for Mac and Linux signs agents in to websites")
	}
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return s.GetAgentSignIn(), err
	}
	s.state.IsAgentSignInEnabled = isEnabled
	err := s.state.Save()
	s.mutex.Unlock()
	if isEnabled && !driver.HasAccess() {
		driver.RequestAccess()
	}
	return s.GetAgentSignIn(), err
}

func refuseSignIn(reason signin.Reason, format string, arguments ...any) *signin.Refusal {
	return &signin.Refusal{Reason: reason, Message: fmt.Sprintf(format, arguments...)}
}

func convertSignInError(err error) error {
	var refusal *signin.Refusal
	if errors.As(err, &refusal) {
		return cliprotocol.NewError(cliprotocol.ErrorSignInRefused, refusal.Message)
	}
	return convertCommandLineError(err)
}

func (s *Service) requireSignIn() (*signin.Signer, error) {
	settings := s.GetAgentSignIn()
	switch {
	case s.signer == nil:
		return nil, refuseSignIn(signin.ReasonUnavailable, "Signing in to websites works only in Hisar for Mac and Linux.")
	case !settings.IsEnabled:
		return nil, refuseSignIn(signin.ReasonDisabled, "Website sign-in is off in Hisar. Ask the user to turn on Hisar → Settings → Developer tools → Coding agents → Website sign-in.")
	case !settings.HasAccessibility:
		return nil, s.signer.RefuseNoAccess()
	}
	return s.signer, nil
}

// isAgentLoginForOrigin needs s.mutex: an item AutoFill fills, saved for origin under the user's host matching.
func (s *Service) isAgentLoginForOrigin(vaultItem *item.Item, origin signin.Origin) bool {
	return vaultItem.OffersLoginFill() && slices.ContainsFunc(vaultItem.Overview.URLs, func(itemURL string) bool {
		return browserprotocol.IsURLMatch(itemURL, origin.String(), s.state.IsExactHostMatch)
	})
}

// PrepareAgentSignIn observes the window and lists the logins in the agent-visible vaults saved for its origin, sorted
// like the browser extension's list; it reads no secret.
func (bridge *CommandLineBridge) PrepareAgentSignIn(ctx context.Context, request *cliprotocol.Request) (clisocket.AgentSignInTarget, error) {
	s, err := bridge.requestService(ctx)
	if err != nil {
		return clisocket.AgentSignInTarget{}, err
	}
	signer, err := s.requireSignIn()
	if err != nil {
		return clisocket.AgentSignInTarget{}, convertSignInError(err)
	}
	destination, err := signer.Inspect(ctx, signin.Target{PID: request.Window.PID, WindowID: request.Window.WindowID})
	if err != nil {
		return clisocket.AgentSignInTarget{}, convertSignInError(err)
	}
	target := clisocket.AgentSignInTarget{Destination: destination}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); errors.Is(err, ErrLocked) {
		target.IsLocked = true
		return target, nil
	} else if err != nil {
		return clisocket.AgentSignInTarget{}, convertCommandLineError(err)
	}
	logins, err := s.listAgentLogins(ctx, destination.Origin, request.ItemID)
	target.Logins = logins
	return target, convertSignInError(err)
}

// listAgentLogins needs s.mutex: the logins saved for origin in the agent-visible vaults, or only itemID's.
func (s *Service) listAgentLogins(ctx context.Context, origin signin.Origin, itemID string) ([]clisocket.AgentLogin, error) {
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		return nil, convertCommandLineError(err)
	}
	var logins []clisocket.AgentLogin
	for vaultID, items := range s.itemsByVaultID {
		vault := s.vaultsByID[vaultID]
		if vault == nil || !s.isAgentVault(vaultID) {
			continue
		}
		for index := range items {
			vaultItem := &items[index]
			if itemID != "" && vaultItem.ID != itemID {
				continue
			}
			if itemID != "" && !s.isAgentLoginForOrigin(vaultItem, origin) {
				return nil, refuseSignIn(signin.ReasonLoginNotForSite, "“%s” is saved for %s, but the page is %s. Hisar only signs in on a login's saved websites.",
					vaultItem.Overview.Title, describeSavedWebsites(vaultItem), origin)
			}
			if s.isAgentLoginForOrigin(vaultItem, origin) {
				logins = append(logins, clisocket.AgentLogin{
					AgentSecret: clisocket.AgentSecret{VaultID: vaultID, ItemID: vaultItem.ID, VaultName: vault.Name, ItemTitle: vaultItem.Overview.Title, Origin: origin.String()},
					Username:    vaultItem.Overview.Username, Revision: vaultItem.Revision,
				})
			}
		}
	}
	if len(logins) == 0 && itemID != "" {
		return nil, refuseSignIn(signin.ReasonNoLogin, "No login available to agents has the ID %q. Find the login with search_items, or leave out login.", itemID)
	}
	if len(logins) == 0 {
		return nil, refuseSignIn(signin.ReasonNoLogin, "No login in the vaults available to agents is saved for %s. Ask the user to add one in Hisar or make its vault available to agents. Don't ask for the password.", origin)
	}
	slices.SortFunc(logins, func(first, second clisocket.AgentLogin) int {
		if comparison := strings.Compare(strings.ToLower(first.ItemTitle), strings.ToLower(second.ItemTitle)); comparison != 0 {
			return comparison
		}
		return strings.Compare(strings.ToLower(first.Username), strings.ToLower(second.Username))
	})
	return logins, nil
}

func describeSavedWebsites(vaultItem *item.Item) string {
	websites := vaultItem.Websites()
	if len(websites) == 0 {
		return "no website"
	}
	return strings.Join(websites, ", ")
}

// PerformAgentSignIn signs in with the approved login. The signer observes the window again first, and only once it
// still shows the approved destination does it ask for the login, which is then read under s.mutex: refused, reading
// no secret, when the request ended, its authorization is stale or the login changed during the approval. The sign-in
// ends when the request does, the agent session ends or Hisar locks.
func (bridge *CommandLineBridge) PerformAgentSignIn(ctx context.Context, target clisocket.AgentSignInTarget, login clisocket.AgentLogin, authorization clisocket.AgentAuthorization) (cliprotocol.AgentSignInResult, error) {
	s, err := bridge.requestService(ctx)
	if err != nil {
		return cliprotocol.AgentSignInResult{}, err
	}
	signer, err := s.requireSignIn()
	if err != nil {
		return cliprotocol.AgentSignInResult{}, convertSignInError(err)
	}
	signInContext, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mutex.Lock()
	if target.LockedGrantGeneration != 0 {
		authorization = authorization.BindLockedGrantGeneration(&s.lockedGrantGeneration, target.LockedGrantGeneration)
	}
	if err := s.requireAgentAuthorization(authorization); err != nil {
		s.mutex.Unlock()
		return cliprotocol.AgentSignInResult{}, err
	}
	stopCancellingOnLock := s.cancelOnLock(cancel)
	s.mutex.Unlock()
	defer stopCancellingOnLock()
	result, err := signer.Run(signInContext, target.Destination, func(loadContext context.Context) (signin.Login, error) {
		s.mutex.Lock()
		defer s.mutex.Unlock()
		if target.LockedGrantGeneration != 0 {
			if loadContext.Err() != nil {
				return signin.Login{}, loadContext.Err()
			}
			if err := s.requireAgentAuthorization(authorization); err != nil {
				return signin.Login{}, err
			}
			return s.readLockedAgentSignInLogin(target, login)
		}
		return s.readAgentSignInLogin(loadContext, authorization, target.Destination.Origin, login)
	})
	described := describeAgentSignIn(target.Destination, login, result)
	if !s.isSessionEpoch(*s.requestEpoch) || !authorization.IsCurrent() {
		return described, errCommandLineSessionEnded
	}
	if err != nil {
		return cliprotocol.AgentSignInResult{}, convertSignInError(err)
	}
	return described, nil
}

// readAgentSignInLogin needs s.mutex: the login as it is now, if the request still runs, its authorization holds and
// the login is still the approved one.
func (s *Service) readAgentSignInLogin(ctx context.Context, authorization clisocket.AgentAuthorization, origin signin.Origin, login clisocket.AgentLogin) (signin.Login, error) {
	if ctx.Err() != nil {
		return signin.Login{}, refuseSignIn(signin.ReasonInterrupted, "The sign-in request ended before Hisar entered anything. Call sign_in again if the agent still needs it.")
	}
	if err := s.requireAgentAuthorization(authorization); err != nil {
		return signin.Login{}, err
	}
	if err := s.requireUnlocked(); err != nil {
		return signin.Login{}, convertCommandLineError(err)
	}
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		return signin.Login{}, convertCommandLineError(err)
	}
	vaultItem, err := s.findItem(login.VaultID, login.ItemID)
	if err != nil || !s.isAgentVault(login.VaultID) || vaultItem.Revision != login.Revision || !s.isAgentLoginForOrigin(vaultItem, origin) {
		return signin.Login{}, refuseSignIn(signin.ReasonChanged, "“%s” changed while the user was approving, or its vault is no longer available to agents. Hisar entered nothing. Call sign_in again.", login.ItemTitle)
	}
	return signin.Login{
		Username: vaultItem.Overview.Username, Password: signin.NewSecret(vaultItem.Details.Password), OneTimePassword: signin.NewSecret(vaultItem.Details.OneTimePassword),
	}, nil
}

// cancelOnLock needs s.mutex: cancel runs when Hisar locks, until the returned function, which takes s.mutex, is
// called.
func (s *Service) cancelOnLock(cancel context.CancelFunc) func() {
	if s.agentSignInCancels == nil {
		s.agentSignInCancels = map[uint64]context.CancelFunc{}
	}
	s.lastAgentSignInID++
	signInID := s.lastAgentSignInID
	s.agentSignInCancels[signInID] = cancel
	return func() {
		s.mutex.Lock()
		defer s.mutex.Unlock()
		delete(s.agentSignInCancels, signInID)
	}
}

// cancelAgentSignIns needs s.mutex; lockSession calls it.
func (s *Service) cancelAgentSignIns() {
	for _, cancel := range s.agentSignInCancels {
		cancel()
	}
	clear(s.agentSignInCancels)
}

func describeAgentSignIn(destination signin.Destination, login clisocket.AgentLogin, result signin.Result) cliprotocol.AgentSignInResult {
	website := destination.Origin.String()
	described := cliprotocol.AgentSignInResult{
		Outcome: string(result.Outcome), Cleanup: string(result.Cleanup), Website: website, Browser: destination.Browser.Name,
		Login: cliprotocol.AgentSignInLogin{ID: login.ItemID, VaultID: login.VaultID, VaultName: login.VaultName, Title: login.ItemTitle, Username: login.Username},
		Steps: make([]cliprotocol.AgentSignInStep, 0, len(result.Steps)),
	}
	var lastParts []string
	for _, step := range result.Steps {
		parts := make([]string, 0, len(step.Filled))
		for _, part := range step.Filled {
			parts = append(parts, string(part))
		}
		described.Steps = append(described.Steps, cliprotocol.AgentSignInStep{Filled: parts, IsSubmitted: step.IsSubmitted})
		if step.IsSubmitted {
			lastParts = parts
		}
	}
	switch result.Outcome {
	case signin.Submitted:
		described.Message = fmt.Sprintf("Hisar signed in to %s as %s and submitted it. Take a screenshot to see the page.", website, login.Username)
	case signin.NotAccepted:
		described.Message = fmt.Sprintf("The site kept its sign-in form after Hisar submitted %s's %s, so it may be wrong or the site refused it. %s Tell the user; do not retry.",
			login.Username, describeParts(lastParts), describeCleanup(result.Cleanup))
	case signin.Stopped:
		described.Message = fmt.Sprintf("Hisar submitted the %s and stopped: %s. Take a screenshot, handle the page, and call sign_in again if a username, password or one-time code field shows.",
			describeParts(lastParts), result.Detail)
	case signin.CleanupFailed:
		described.Message = fmt.Sprintf("Hisar entered %s's login on %s but could not confirm it cleared it from the page. Stop, and tell the user to check the browser window. Do not call sign_in again.",
			login.Username, website)
	}
	return described
}

func describeCleanup(cleanup signin.Cleanup) string {
	switch cleanup {
	case signin.CleanupPageLeft:
		return "The page Hisar entered it on is gone."
	case signin.CleanupFieldsRemoved:
		return "The page removed the fields Hisar filled itself."
	}
	return "Hisar cleared what it entered."
}

func describeParts(parts []string) string {
	switch len(parts) {
	case 0:
		return "sign-in"
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

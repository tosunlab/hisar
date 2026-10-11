package vault

import (
	"log"

	"github.com/tryhisar/hisar/desktop/background"
)

// LoginItemStatus is Settings → General → "Open Hisar at login": the switch and what the system reports.
type LoginItemStatus struct {
	IsAvailable           bool   `json:"isAvailable"`
	IsOpenAtLogin         bool   `json:"isOpenAtLogin"`
	SystemState           string `json:"systemState"`
	CanOpenSystemSettings bool   `json:"canOpenSystemSettings"`
	ErrorMessage          string `json:"errorMessage"`
}

// LoginItemService is bound to the UI for the "Open Hisar at login" switch.
type LoginItemService struct {
	service   *Service
	loginItem background.LoginItem
}

func CreateLoginItemService(service *Service, loginItem background.LoginItem) *LoginItemService {
	return &LoginItemService{service: service, loginItem: loginItem}
}

// ApplyOpenAtLoginOnLaunch registers the login item on the first launch of a build that has the switch, and never again
// on its own; not a method, so the UI cannot call it.
func ApplyOpenAtLoginOnLaunch(loginItemService *LoginItemService) error {
	if state, err := loginItemService.loginItem.GetState(); err != nil || state == background.LoginItemUnavailable {
		return err
	}
	s := loginItemService.service
	s.mutex.Lock()
	isConfigured, isDisabled := s.state.IsOpenAtLoginConfigured, s.state.IsOpenAtLoginDisabled
	s.mutex.Unlock()
	if isConfigured {
		return nil
	}
	var registerError error
	if !isDisabled {
		registerError = loginItemService.loginItem.Register()
		if registerError != nil {
			log.Printf("open at login: %v", registerError)
		}
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.IsOpenAtLoginConfigured = true
	if err := s.state.Save(); err != nil {
		return err
	}
	return registerError
}

func (loginItemService *LoginItemService) GetLoginItemStatus() LoginItemStatus {
	return loginItemService.buildStatus(nil)
}

func (loginItemService *LoginItemService) buildStatus(changeError error) LoginItemStatus {
	state, err := loginItemService.loginItem.GetState()
	s := loginItemService.service
	s.mutex.Lock()
	isOpenAtLogin := !s.state.IsOpenAtLoginDisabled
	s.mutex.Unlock()
	status := LoginItemStatus{
		IsAvailable:           state != background.LoginItemUnavailable,
		IsOpenAtLogin:         isOpenAtLogin && state != background.LoginItemUnavailable,
		SystemState:           string(state),
		CanOpenSystemSettings: loginItemService.loginItem.CanOpenSystemSettings(),
	}
	if changeError == nil {
		changeError = err
	}
	if changeError != nil {
		status.ErrorMessage = changeError.Error()
	}
	return status
}

// SetOpenAtLogin saves the switch and registers or unregisters the login item; the status carries any system error.
func (loginItemService *LoginItemService) SetOpenAtLogin(isEnabled bool) (LoginItemStatus, error) {
	if state, _ := loginItemService.loginItem.GetState(); state == background.LoginItemUnavailable {
		return loginItemService.buildStatus(nil), background.ErrUnavailable
	}
	s := loginItemService.service
	s.mutex.Lock()
	s.state.IsOpenAtLoginDisabled = !isEnabled
	s.state.IsOpenAtLoginConfigured = true
	saveError := s.state.Save()
	s.mutex.Unlock()
	if saveError != nil {
		return loginItemService.buildStatus(saveError), saveError
	}
	var changeError error
	if isEnabled {
		changeError = loginItemService.loginItem.Register()
	} else {
		changeError = loginItemService.loginItem.Unregister()
	}
	return loginItemService.buildStatus(changeError), nil
}

func (loginItemService *LoginItemService) OpenLoginItemSettings() error {
	return loginItemService.loginItem.OpenSystemSettings()
}

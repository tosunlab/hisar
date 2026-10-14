// Package loginitem is Settings → General → "Open Hisar at login", bound to the UI.
package loginitem

import (
	"log"

	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/desktop/background"
	"github.com/tryhisar/hisar/desktop/vault"
)

// Status is the switch and what the system reports.
type Status struct {
	IsAvailable           bool   `json:"isAvailable"`
	IsOpenAtLogin         bool   `json:"isOpenAtLogin"`
	SystemState           string `json:"systemState"`
	CanOpenSystemSettings bool   `json:"canOpenSystemSettings"`
	ErrorMessage          string `json:"errorMessage"`
}

// Service is bound to the UI for the "Open Hisar at login" switch.
type Service struct {
	appService *vault.Service
	loginItem  background.LoginItem
}

func NewService(appService *vault.Service, loginItem background.LoginItem) *Service {
	return &Service{appService: appService, loginItem: loginItem}
}

// ApplyOnLaunch registers the login item on the first launch of a build that has the switch, and never again
// on its own; not a method, so the UI cannot call it.
func ApplyOnLaunch(service *Service) error {
	if state, err := service.loginItem.GetState(); err != nil || state == background.LoginItemUnavailable {
		return err
	}
	settings := vault.ReadSettings(service.appService)
	if settings.IsOpenAtLoginConfigured {
		return nil
	}
	var registerError error
	if !settings.IsOpenAtLoginDisabled {
		registerError = service.loginItem.Register()
		if registerError != nil {
			log.Printf("open at login: %v", registerError)
		}
	}
	if err := vault.UpdateSettings(service.appService, func(settings *localstate.Settings) { settings.IsOpenAtLoginConfigured = true }); err != nil {
		return err
	}
	return registerError
}

func (service *Service) GetLoginItemStatus() Status {
	return service.buildStatus(nil)
}

func (service *Service) buildStatus(changeError error) Status {
	state, err := service.loginItem.GetState()
	isOpenAtLogin := !vault.ReadSettings(service.appService).IsOpenAtLoginDisabled
	status := Status{
		IsAvailable:           state != background.LoginItemUnavailable,
		IsOpenAtLogin:         isOpenAtLogin && state != background.LoginItemUnavailable,
		SystemState:           string(state),
		CanOpenSystemSettings: service.loginItem.CanOpenSystemSettings(),
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
func (service *Service) SetOpenAtLogin(isEnabled bool) (Status, error) {
	if state, _ := service.loginItem.GetState(); state == background.LoginItemUnavailable {
		return service.buildStatus(nil), background.ErrUnavailable
	}
	saveError := vault.UpdateSettings(service.appService, func(settings *localstate.Settings) {
		settings.IsOpenAtLoginDisabled = !isEnabled
		settings.IsOpenAtLoginConfigured = true
	})
	if saveError != nil {
		return service.buildStatus(saveError), saveError
	}
	var changeError error
	if isEnabled {
		changeError = service.loginItem.Register()
	} else {
		changeError = service.loginItem.Unregister()
	}
	return service.buildStatus(changeError), nil
}

func (service *Service) OpenLoginItemSettings() error {
	return service.loginItem.OpenSystemSettings()
}

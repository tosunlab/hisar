package main

import (
	"errors"
	"fmt"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
)

// loadState refuses standalone mode inside a harness; behind a sandbox that blocks the app's socket the sandbox is what
// stopped hisar, so it says that instead.
func loadState() (*localstate.State, error) {
	if err := checkStandaloneAllowed(); err != nil {
		if errors.Is(selectedMode.appError, errSocketBlocked) {
			return nil, errSocketBlocked
		}
		return nil, err
	}
	path, err := localstate.GetPath("HISAR_STATE", "state.json")
	if err != nil {
		return nil, err
	}
	return localstate.Load(path)
}

func requireLogin(currentState *localstate.State) error {
	if !currentState.IsLoggedIn() && selectedMode.appError != nil {
		return selectedMode.appError
	}
	if !currentState.IsLoggedIn() {
		return errors.New("not logged in; run `hisar login` or `hisar signup <invite-link>`")
	}
	return nil
}

func loadSecretKey(accountID string) (crypt.SecretKey, error) {
	secretKey, err := localstate.LoadSecretKey(accountID)
	if err != nil {
		return secretKey, fmt.Errorf("%w (set HISAR_SECRET_KEY or log in again)", err)
	}
	return secretKey, nil
}

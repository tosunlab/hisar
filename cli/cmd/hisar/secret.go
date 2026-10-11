package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/secretreference"
	"github.com/tryhisar/hisar/core/session"
	"golang.org/x/term"
)

var isStandaloneForcedByOption bool

type sessionSecretSource struct {
	ctx          context.Context
	session      *session.Session
	allVaultByID map[string]*session.Vault
}

func (source *sessionSecretSource) ListAllVault() ([]secretreference.Vault, error) {
	allSessionVault, err := source.session.ListAllVault(source.ctx)
	if err != nil {
		return nil, err
	}
	source.allVaultByID = map[string]*session.Vault{}
	allVault := make([]secretreference.Vault, 0, len(allSessionVault))
	for index := range allSessionVault {
		source.allVaultByID[allSessionVault[index].ID] = &allSessionVault[index]
		allVault = append(allVault, secretreference.Vault{ID: allSessionVault[index].ID, Name: allSessionVault[index].Name})
	}
	return allVault, nil
}

func (source *sessionSecretSource) ListAllItem(vaultID string) ([]item.Item, error) {
	vault, isFound := source.allVaultByID[vaultID]
	if !isFound {
		return nil, fmt.Errorf("vault %q: not found", vaultID)
	}
	return source.session.ListAllItem(source.ctx, vault)
}

// checkAllReference parses every reference before anything is asked, so a typo never opens an approval.
func checkAllReference(allReference []string) error {
	var allFailure []error
	for _, text := range allReference {
		if _, err := secretreference.Parse(text); err != nil {
			allFailure = append(allFailure, &secretreference.Error{Reference: text, Reason: err.Error()})
		}
	}
	return errors.Join(allFailure...)
}

// resolveAllReference asks the app for every reference in one request, or resolves them standalone; the values
// come back in the order of allReference.
func resolveAllReference(ctx context.Context, allReference []string, commandLine []string) ([]string, error) {
	if err := checkAllReference(allReference); err != nil {
		return nil, err
	}
	if len(allReference) > cliprotocol.MaxReferenceCount {
		return nil, fmt.Errorf("at most %d different secret references can be used at once", cliprotocol.MaxReferenceCount)
	}
	mode := chooseMode(isStandaloneForcedByOption)
	selectedMode = mode
	if mode.client != nil {
		defer mode.client.Close()
		var result cliprotocol.ResolveSecretResult
		if err := mode.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: allReference, CommandLine: commandLine}, &result); err != nil {
			return nil, err
		}
		if len(result.Values) != len(allReference) {
			return nil, errors.New("the Hisar app answered with the wrong number of values")
		}
		return result.Values, nil
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return nil, err
	}
	var allValue []string
	err = func() error {
		allResolved, err := secretreference.ResolveAll(&sessionSecretSource{ctx: ctx, session: unlockedSession}, allReference, time.Now())
		if err != nil {
			return err
		}
		for _, resolved := range allResolved {
			allValue = append(allValue, resolved.Value)
		}
		return nil
	}()
	return allValue, finishSession(currentState, unlockedSession, err)
}

func runRead(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	isNewlineOmitted := flags.Bool("no-newline", false, "never end the value with a newline")
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	if len(allPositional) != 1 {
		return errUsage
	}
	if !secretreference.IsReference(allPositional[0]) {
		return fmt.Errorf("%q is not a secret reference; use hisar://<vault>/<item>/<field>", allPositional[0])
	}
	allValue, err := resolveAllReference(ctx, allPositional, nil)
	if err != nil {
		return err
	}
	output := allValue[0]
	if !*isNewlineOmitted && term.IsTerminal(int(os.Stdout.Fd())) {
		output += "\n"
	}
	_, err = os.Stdout.WriteString(output)
	return err
}

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/secretreference"
	"github.com/tryhisar/hisar/core/session"
	"golang.org/x/term"
)

var isStandaloneForcedByOption bool

type sessionSecretSource struct {
	ctx        context.Context
	session    *session.Session
	vaultsByID map[string]*session.Vault
}

func (source *sessionSecretSource) ListVaults() (secretreference.Inventory, error) {
	read, err := source.session.ReadVaults(source.ctx)
	if err != nil {
		return secretreference.Inventory{}, err
	}
	source.vaultsByID = map[string]*session.Vault{}
	inventory := secretreference.Inventory{Vaults: make([]secretreference.Vault, 0, len(read.Vaults)), IsComplete: len(read.Issues) == 0}
	for index := range read.Vaults {
		source.vaultsByID[read.Vaults[index].ID] = &read.Vaults[index]
		inventory.Vaults = append(inventory.Vaults, secretreference.Vault{ID: read.Vaults[index].ID, Name: read.Vaults[index].Name})
	}
	return inventory, nil
}

func (source *sessionSecretSource) ListItems(vaultID string) (secretreference.ItemList, error) {
	vault, isFound := source.vaultsByID[vaultID]
	if !isFound {
		return secretreference.ItemList{}, fmt.Errorf("vault %q: not found", vaultID)
	}
	read, err := source.session.ReadItems(source.ctx, vault)
	if err != nil {
		return secretreference.ItemList{}, err
	}
	return secretreference.ItemList{Items: read.Items, IsComplete: len(read.Issues) == 0 && read.Provenance.HasCompletedSync}, nil
}

// checkReferences parses every reference before anything is asked, so a typo never opens an approval.
func checkReferences(references []string) error {
	var failures []error
	for _, text := range references {
		if _, err := secretreference.Parse(text); err != nil {
			failures = append(failures, &secretreference.Error{Reference: text, Reason: err.Error()})
		}
	}
	return errors.Join(failures...)
}

// resolveReferences asks the app for every reference in one request, or resolves them standalone; the values
// come back in the order of references.
func resolveReferences(ctx context.Context, references []string, commandLine []string) ([]string, error) {
	if err := checkReferences(references); err != nil {
		return nil, err
	}
	if len(references) > cliprotocol.MaxReferenceCount {
		return nil, fmt.Errorf("at most %d different secret references can be used at once", cliprotocol.MaxReferenceCount)
	}
	mode := chooseMode(isStandaloneForcedByOption)
	selectedMode = mode
	if mode.client != nil {
		defer mode.client.Close()
		var result cliprotocol.ResolveSecretResult
		if err := mode.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: references, CommandLine: commandLine}, &result); err != nil {
			return nil, err
		}
		if len(result.Values) != len(references) {
			return nil, errors.New("the Hisar app answered with the wrong number of values")
		}
		return result.Values, nil
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return nil, err
	}
	var values []string
	err = func() error {
		resolvedReferences, err := secretreference.ResolveAll(&sessionSecretSource{ctx: ctx, session: unlockedSession}, references, time.Now())
		if err != nil {
			return err
		}
		for _, resolved := range resolvedReferences {
			values = append(values, resolved.Value)
		}
		return nil
	}()
	return values, finishSession(currentState, unlockedSession, err)
}

func runRead(ctx context.Context, flags *flag.FlagSet, args []string) error {
	isNewlineOmitted := flags.Bool("no-newline", false, "never end the value with a newline")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	if !secretreference.IsReference(positionals[0]) {
		return fmt.Errorf("%q is not a secret reference; use hisar://<vault>/<item>/<field>", positionals[0])
	}
	values, err := resolveReferences(ctx, positionals, nil)
	if err != nil {
		return err
	}
	output := values[0]
	if !*isNewlineOmitted && term.IsTerminal(int(os.Stdout.Fd())) {
		output += "\n"
	}
	_, err = os.Stdout.WriteString(output)
	return err
}

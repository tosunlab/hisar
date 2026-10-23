package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/localstate"
)

type recoveryFileSaver string

func (path recoveryFileSaver) ChooseSavePath(string) (string, error) { return string(path), nil }

func newRecoveryService(outputPath string) (*app.Service, error) {
	if _, err := loadState(); err != nil {
		return nil, err
	}
	path, err := localstate.GetPath("HISAR_STATE", "state.json")
	if err != nil {
		return nil, err
	}
	device := getDeviceInfo()
	return app.NewService(app.Config{StatePath: path, DeviceName: device.Name, Platform: device.Platform, AppVersion: version, IsEventStreamDisabled: true, FileSaver: recoveryFileSaver(outputPath)})
}

func printPendingRecovery(operationID, phase string, problems []string) {
	fmt.Fprintf(standardError(), "Recovery operation %s is %s.\n", operationID, phase)
	if phase == "awaiting-kit" {
		fmt.Fprintln(standardError(), "No credential finish was sent. Save and acknowledge the Emergency Kit before explicitly resuming.")
	}
	for _, problem := range problems {
		fmt.Fprintln(standardError(), escapeControlCharacters(problem))
	}
	fmt.Fprintf(standardError(), "Use `hisar account pending kit %s -o <new-file>` to save the kit, `hisar account pending acknowledge %s` to acknowledge it, and `hisar account pending resume %s` to finish. Keep both old and new kits until the remote outcome is known.\n", operationID, operationID, operationID)
}

func runPendingRecovery(_ context.Context, flags *flag.FlagSet, args []string) error {
	action := "list"
	if len(args) > 0 {
		action, args = args[0], args[1:]
	}
	output := flags.String("o", "Hisar Emergency Kit.pdf", "where to save the pending Emergency Kit PDF")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	service, err := newRecoveryService(*output)
	if err != nil {
		return err
	}
	if action == "list" {
		if len(positionals) != 0 {
			return errUsage
		}
		entries, err := service.ListPendingRecovery()
		if err != nil {
			return err
		}
		for _, entry := range entries {
			fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\n", entry.OperationID, escapeControlCharacters(entry.Origin), escapeControlCharacters(entry.Email), entry.Phase)
		}
		return nil
	}
	if len(positionals) != 1 {
		return errors.New("choose a pending operation ID from `hisar account pending list`")
	}
	password, err := promptSecret("Master password used for this operation")
	if err != nil {
		return err
	}
	defer clear(password)
	id := positionals[0]
	var kitKey, priorKey []byte
	defer func() { clear(kitKey); clear(priorKey) }()
	if action == "resume" || action == "kit" {
		entries, err := service.ListPendingRecovery()
		if err != nil {
			return err
		}
		var selected *app.PendingRecoverySummary
		for index := range entries {
			if entries[index].OperationID == id {
				selected = &entries[index]
				break
			}
		}
		if selected == nil {
			return errors.New("choose a pending operation ID from `hisar account pending list`")
		}
		if !selected.KeychainProtected {
			kitKey, err = promptSecret("Secret Key from this operation's saved Emergency Kit")
			if err != nil {
				return err
			}
			if action == "resume" && selected.Kind == "replace-secret-key" {
				if key, err := localstate.LoadSecretKey(selected.AccountID); err == nil {
					priorKey = []byte(key.Encode())
				} else {
					priorKey, err = promptSecret("Previous Secret Key (leave empty if the original password check is still valid)")
					if err != nil {
						return err
					}
				}
			}
		}
	}
	switch action {
	case "resume":
		result, err := service.ResumePendingRecoveryWithSecretKey(id, string(password), string(kitKey), string(priorKey))
		if err != nil {
			return err
		}
		printPendingRecovery(result.PendingOperationID, result.SetupState, result.Problems)
		return nil
	case "kit":
		if len(kitKey) > 0 {
			if err := app.OpenPendingRecoveryForDelivery(service, id, string(password), string(kitKey)); err != nil {
				return err
			}
		}
		path, err := service.SavePendingRecoveryEmergencyKit(id, string(password))
		if err != nil {
			return err
		}
		fmt.Fprintf(standardError(), "Saved the pending Emergency Kit to %s. Keep it safe before acknowledging delivery.\n", filepath.Clean(path))
		return nil
	case "acknowledge":
		if err := confirmUnlessYes(false, "Have you saved this operation's Emergency Kit somewhere safe?"); err != nil {
			return err
		}
		return service.AcknowledgePendingRecovery(id, string(password))
	case "discard":
		if err := confirmUnlessYes(false, "Discard this prepared operation only if no remote credential finish was sent?"); err != nil {
			return err
		}
		return service.DiscardPreparedRecovery(id, string(password))
	}
	return errors.New("use pending list, resume, kit, acknowledge or discard")
}

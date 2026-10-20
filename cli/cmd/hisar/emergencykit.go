package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/tryhisar/hisar/core/emergencykit"
)

func runEmergencyKit(ctx context.Context, flags *flag.FlagSet, args []string) error {
	outputPath := flags.String("o", "Hisar Emergency Kit.pdf", "where to save the PDF")
	if _, err := parse(flags, args); err != nil {
		return err
	}
	if _, err := os.Stat(*outputPath); err == nil {
		return fmt.Errorf("%s already exists; choose another file with -o", *outputPath)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		secretKey, err := loadSecretKey(currentState.AccountID)
		if err != nil {
			return err
		}
		document, err := emergencykit.Generate(emergencykit.Kit{
			Name:      unlockedSession.Name,
			Email:     currentState.Email,
			Server:    currentState.Server,
			SecretKey: secretKey.Encode(),
			CreatedAt: time.Now(),
		})
		if err != nil {
			return err
		}
		if err := writeNewPrivateFile(*outputPath, document); err != nil {
			return err
		}
		fmt.Fprintf(standardError(), "Saved your Emergency Kit to %s. Print it, then delete the file.\n", *outputPath)
		return nil
	}()
	return finishSession(currentState, unlockedSession, err)
}

// writeNewPrivateFile creates the file with mode 0600 and never replaces an existing one.
func writeNewPrivateFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	return file.Close()
}

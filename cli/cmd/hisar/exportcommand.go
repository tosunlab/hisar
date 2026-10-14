package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/export"
)

const plainExportWarning = "Plain JSON is not encrypted: anyone who gets the file can read every password, one-time code secret and SSH private key in it."

func runExport(ctx context.Context, flags *flag.FlagSet, args []string) error {
	access := &standaloneBackend{}
	return access.finish(runExportWithAccess(ctx, access, flags, args))
}

func runExportThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	return runExportWithAccess(ctx, appBackend{client: client}, flags, args)
}

func buildDefaultExportPath(format string, now time.Time) string {
	if format == cliprotocol.ExportFormatPlain {
		return "hisar-export-" + now.Format("2006-01-02") + ".json"
	}
	return "hisar-export-" + now.Format("2006-01-02") + ".hisar"
}

func chooseExportFormat() (string, error) {
	fmt.Fprintln(os.Stderr, "Format:\n  1) Encrypted export, protected by an export passphrase (recommended)\n  2) Plain JSON, not encrypted")
	for {
		answer, err := prompt("Choose", "1")
		if err != nil {
			return "", err
		}
		switch answer {
		case "1":
			return cliprotocol.ExportFormatEncrypted, nil
		case "2":
			return cliprotocol.ExportFormatPlain, nil
		}
		fmt.Fprintln(os.Stderr, "Enter 1 or 2.")
	}
}

// promptExportPassphrase asks twice on a terminal; a script pipes the passphrase on one line.
func promptExportPassphrase() ([]byte, error) {
	isInteractive := isInteractiveTerminal()
	for {
		passphrase, err := promptSecret("Export passphrase")
		if err != nil {
			return nil, err
		}
		if err := export.CheckPassphrase(passphrase); err != nil {
			if !isInteractive {
				return nil, err
			}
			fmt.Fprintf(os.Stderr, "Use at least %d characters.\n", export.MinPassphraseLength)
			continue
		}
		if !isInteractive {
			return passphrase, nil
		}
		repeated, err := promptSecret("Repeat export passphrase")
		if err != nil {
			return nil, err
		}
		isSame := string(passphrase) == string(repeated)
		clear(repeated)
		if isSame {
			return passphrase, nil
		}
		clear(passphrase)
		fmt.Fprintln(os.Stderr, "The passphrases do not match.")
	}
}

func runExportWithAccess(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	outputPath := flags.String("o", "", "the file to write; it must not exist yet (default hisar-export-<date>.hisar or .json)")
	format := flags.String("format", "", "encrypted (default) or plain")
	isConfirmed := flags.Bool("y", false, "write a plain JSON export without asking")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 0 {
		return errUsage
	}
	isInteractive := isInteractiveTerminal()
	switch *format {
	case "":
		*format = cliprotocol.ExportFormatEncrypted
		if isInteractive {
			if *format, err = chooseExportFormat(); err != nil {
				return err
			}
		}
	case cliprotocol.ExportFormatEncrypted, cliprotocol.ExportFormatPlain:
	default:
		return fmt.Errorf("unknown format %q; use encrypted or plain", *format)
	}
	if *outputPath == "" {
		*outputPath = buildDefaultExportPath(*format, time.Now())
	}
	absolutePath, err := filepath.Abs(*outputPath)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(absolutePath); err == nil {
		return fmt.Errorf("%s already exists; choose another file with -o", *outputPath)
	}

	change := cliprotocol.ExportChange{Format: *format, Path: absolutePath}
	if *format == cliprotocol.ExportFormatPlain {
		fmt.Fprintln(os.Stderr, plainExportWarning)
		if !*isConfirmed {
			if !isInteractive {
				return errNeedsConfirmation
			}
			isWriting, err := confirm(fmt.Sprintf("Write every secret in plain text to %s?", *outputPath))
			if err != nil || !isWriting {
				return err
			}
		}
	} else {
		passphrase, err := promptExportPassphrase()
		if err != nil {
			return err
		}
		change.Passphrase = string(passphrase)
		clear(passphrase)
	}
	result, err := access.writeExport(ctx, change)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Exported %s from %s to %s.\n", formatItemCount(result.ItemCount), formatVaultCount(result.VaultCount), *outputPath)
	if *format == cliprotocol.ExportFormatPlain {
		fmt.Fprintln(os.Stderr, "It is not encrypted; delete it when you are done.")
	} else {
		fmt.Fprintln(os.Stderr, "Keep the export passphrase somewhere safe: nobody, including Hisar, can open the file without it.")
	}
	return nil
}

func formatVaultCount(count int) string {
	if count == 1 {
		return "1 vault"
	}
	return fmt.Sprintf("%d vaults", count)
}

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/cli/internal/dotenv"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/secretreference"
)

const dotenvFormat = "dotenv"

type dotenvImportOption struct {
	vaultName     string
	isVaultChosen bool
	itemTitle     string
	chosenKeys    []string
	isDryRun      bool
	isConfirmed   bool
}

// dotenvPlan is what an import stores and how the file changes; nothing is written while it is only planned.
type dotenvPlan struct {
	path             string
	source           dotenvSource
	layout           *dotenv.Layout
	storedVariables  []dotenv.LayoutVariable
	keptNames        []string
	vault            cliprotocol.Vault
	itemTitle        string
	existingItemID   string
	referencesByName map[string]string
}

type dotenvSource struct {
	targetPath string
	info       os.FileInfo
	digest     [sha256.Size]byte
}

func readDotenvSource(path string) ([]byte, dotenvSource, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, dotenvSource{}, err
	}
	targetPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return nil, dotenvSource{}, err
	}
	file, err := os.Open(targetPath)
	if err != nil {
		return nil, dotenvSource{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, dotenvSource{}, err
	}
	if !info.Mode().IsRegular() {
		return nil, dotenvSource{}, errors.New("choose a regular .env file")
	}
	content, err := io.ReadAll(io.LimitReader(file, maxImportFileSize+1))
	if err != nil || len(content) > maxImportFileSize {
		clear(content)
		return nil, dotenvSource{}, errors.New("could not read the .env file within the 100 MB limit")
	}
	return content, dotenvSource{targetPath: targetPath, info: info, digest: sha256.Sum256(content)}, nil
}

func checkDotenvSource(path string, source dotenvSource) error {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	targetPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return err
	}
	if targetPath != source.targetPath {
		return errors.New("the .env target changed since the import was planned; it was not rewritten")
	}
	file, err := os.Open(targetPath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Mode() != source.info.Mode() {
		return errors.New("the .env file changed since the import was planned; it was not rewritten")
	}
	digest := sha256.New()
	size, err := io.Copy(digest, io.LimitReader(file, maxImportFileSize+1))
	if err != nil {
		return err
	}
	var currentDigest [sha256.Size]byte
	digest.Sum(currentDigest[:0])
	if size > maxImportFileSize || currentDigest != source.digest {
		return errors.New("the .env file changed since the import was planned; it was not rewritten")
	}
	return nil
}

func isDotenvFile(path, format string) bool {
	if format == dotenvFormat {
		return true
	}
	name := filepath.Base(path)
	return format == "" && (name == ".env" || strings.HasPrefix(name, ".env.") || strings.HasSuffix(name, ".env"))
}

func buildDefaultItemTitle(path string) string {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		absolutePath = path
	}
	title := filepath.Base(filepath.Dir(absolutePath))
	if environment, isNamed := strings.CutPrefix(filepath.Base(path), ".env."); isNamed && environment != "" {
		title += " " + environment
	}
	if title == "" || title == "/" || title == "." {
		return "Environment"
	}
	return title
}

func checkStorableName(name string) error {
	builtInField, isBuiltIn := secretreference.GetBuiltInField(name)
	if !isBuiltIn {
		return nil
	}
	switch builtInField {
	case secretreference.FieldPassword, secretreference.FieldUsername, secretreference.FieldNotes, secretreference.FieldURL:
		return nil
	}
	fieldDescription := map[string]string{secretreference.FieldOneTimeCode: "one-time code", secretreference.FieldFingerprint: "SSH key fingerprint"}[builtInField]
	if fieldDescription == "" {
		fieldDescription = builtInField
	}
	return fmt.Errorf("%s can't be stored: in a secret reference “%s” means the item's %s; rename the variable or keep it as it is", name, strings.ToLower(name), fieldDescription)
}

func chooseDotenvVariable(storable []dotenv.LayoutVariable, chosenKeys []string) (map[string]bool, error) {
	chosenByName := map[string]bool{}
	if chosenKeys != nil {
		for _, key := range chosenKeys {
			if !slices.ContainsFunc(storable, func(variable dotenv.LayoutVariable) bool { return variable.Name == key }) {
				return nil, fmt.Errorf("--keys names %s, which is not a variable with a value in the file", key)
			}
			chosenByName[key] = true
		}
		return chosenByName, nil
	}
	for _, variable := range storable {
		chosenByName[variable.Name] = variable.IsSecretLooking
	}
	return chosenByName, nil
}

func printDotenvChoice(storable []dotenv.LayoutVariable, chosenByName map[string]bool) {
	nameWidth := 0
	for _, variable := range storable {
		nameWidth = max(nameWidth, len(variable.Name))
	}
	for index, variable := range storable {
		mark, description := " ", "stays a plain value ("+variable.Reason+")"
		if chosenByName[variable.Name] {
			mark, description = "x", "store in Hisar"
		}
		fmt.Fprintf(standardError(), "  [%s] %2d  %-*s  %s\n", mark, index+1, nameWidth, variable.Name, description)
	}
}

// toggleDotenvChoice lets the user switch variables by number until they press Return.
func toggleDotenvChoice(storable []dotenv.LayoutVariable, chosenByName map[string]bool) error {
	for {
		printDotenvChoice(storable, chosenByName)
		answer, err := prompt("Type numbers to switch them (for example 2 5), or press Return to continue", "")
		if err != nil {
			return err
		}
		if answer == "" {
			return nil
		}
		for _, field := range strings.FieldsFunc(answer, func(character rune) bool { return character == ' ' || character == ',' }) {
			number, err := strconv.Atoi(field)
			if err != nil || number < 1 || number > len(storable) {
				fmt.Fprintf(standardError(), "%s is not a number from 1 to %d.\n", field, len(storable))
				continue
			}
			name := storable[number-1].Name
			chosenByName[name] = !chosenByName[name]
		}
	}
}

func chooseDotenvVault(ctx context.Context, access backend) (cliprotocol.Vault, error) {
	vaults, err := access.listVaults(ctx)
	if err != nil {
		return cliprotocol.Vault{}, err
	}
	writable := slices.DeleteFunc(vaults, func(vault cliprotocol.Vault) bool {
		return !apitypes.CanWriteItems(vault.Role)
	})
	if len(writable) == 0 {
		return cliprotocol.Vault{}, errors.New("you have no vault you can add items to")
	}
	fmt.Fprintln(standardError(), "Vault:")
	for index, vault := range writable {
		fmt.Fprintf(standardError(), "  %d) %s\n", index+1, vault.Name)
	}
	for {
		answer, err := prompt("Choose", "1")
		if err != nil {
			return cliprotocol.Vault{}, err
		}
		if number, err := strconv.Atoi(answer); err == nil && number >= 1 && number <= len(writable) {
			return writable[number-1], nil
		}
		for _, vault := range writable {
			if strings.EqualFold(vault.Name, answer) {
				return vault, nil
			}
		}
		fmt.Fprintf(standardError(), "Enter a number from 1 to %d.\n", len(writable))
	}
}

// buildReferenceVaultSegment names the vault, or gives its ID when the name would also find another vault: another
// vault of that name, or one whose ID it is, since references try IDs first.
func buildReferenceVaultSegment(vaults []cliprotocol.Vault, vault cliprotocol.Vault) string {
	sameNameCount := 0
	for _, candidate := range vaults {
		if strings.EqualFold(candidate.Name, vault.Name) {
			sameNameCount++
		}
		if candidate.ID == vault.Name && candidate.ID != vault.ID {
			return vault.ID
		}
	}
	if sameNameCount > 1 {
		return vault.ID
	}
	return vault.Name
}

func findDotenvItem(ctx context.Context, access backend, vault cliprotocol.Vault, title string) (string, error) {
	items, err := access.listItems(ctx, vault.ID)
	if err != nil {
		return "", err
	}
	matchIDs := []string{}
	isAnotherItemsID := false
	for _, vaultItem := range items {
		if strings.EqualFold(vaultItem.Overview.Title, title) {
			matchIDs = append(matchIDs, vaultItem.ID)
		} else if vaultItem.ID == title {
			isAnotherItemsID = true
		}
	}
	if isAnotherItemsID {
		return "", fmt.Errorf("%q is the ID of another item in %s; choose another item name", title, vault.Name)
	}
	if len(matchIDs) > 1 {
		return "", fmt.Errorf("%d items in %s are titled %q; choose another item name", len(matchIDs), vault.Name, title)
	}
	if len(matchIDs) == 1 {
		return matchIDs[0], nil
	}
	return "", nil
}

func importDotenv(ctx context.Context, access backend, path string, option dotenvImportOption) error {
	plan, err := planDotenvImport(ctx, access, path, option)
	if err != nil || plan == nil {
		return err
	}
	printDotenvPlan(plan)
	if option.isDryRun {
		fmt.Fprintf(standardError(), "Dry run: nothing was stored and %s was not changed.\n", path)
		return nil
	}
	if !option.isConfirmed {
		if !isInteractiveTerminal() {
			return errNeedsConfirmation
		}
		isContinuing, err := confirm("Continue?")
		if err != nil || !isContinuing {
			return err
		}
	}
	if err := checkDotenvSource(plan.path, plan.source); err != nil {
		return err
	}
	if err := storeDotenvItem(ctx, access, plan); err != nil {
		return err
	}
	if err := rewriteDotenvFile(plan.path, plan.layout.Render(plan.referencesByName), plan.source); err != nil {
		return fmt.Errorf("the values are stored in %s › %s, but %s could not be rewritten: %w", plan.vault.Name, plan.itemTitle, path, err)
	}
	fmt.Fprintf(standardError(), "Stored %s in %s › %s and rewrote %s with secret references.\n", formatValueCount(len(plan.storedVariables)), plan.vault.Name, plan.itemTitle, path)
	fmt.Fprintf(standardError(), "Run a command with them:\n  hisar run --env-file %s -- <command>\n", quoteShellWord(path))
	return nil
}

func planDotenvImport(ctx context.Context, access backend, path string, option dotenvImportOption) (*dotenvPlan, error) {
	content, source, err := readDotenvSource(path)
	if err != nil {
		return nil, err
	}
	layout, err := dotenv.ParseLayout(path, string(content))
	clear(content)
	if err != nil {
		return nil, err
	}
	storable, skippedDescriptions := dotenv.StorableVariables(layout.ListVariables())
	if len(storable) == 0 {
		fmt.Fprintf(standardError(), "%s has no values to store; it was not changed.\n", path)
		return nil, nil
	}
	chosenByName, err := chooseDotenvVariable(storable, option.chosenKeys)
	if err != nil {
		return nil, err
	}
	isInteractive := isInteractiveTerminal()
	if isInteractive && option.chosenKeys == nil {
		fmt.Fprintf(standardError(), "Found %s in %s:\n", formatVariableCount(len(storable)), path)
		if err := toggleDotenvChoice(storable, chosenByName); err != nil {
			return nil, err
		}
	}
	plan := &dotenvPlan{path: path, source: source, layout: layout, referencesByName: map[string]string{}}
	for _, variable := range storable {
		if !chosenByName[variable.Name] {
			plan.keptNames = append(plan.keptNames, variable.Name)
			continue
		}
		if err := checkStorableName(variable.Name); err != nil {
			return nil, err
		}
		plan.storedVariables = append(plan.storedVariables, variable)
	}
	for _, description := range skippedDescriptions {
		plan.keptNames = append(plan.keptNames, description)
	}
	if len(plan.storedVariables) == 0 {
		fmt.Fprintf(standardError(), "No variables chosen; %s was not changed.\n", path)
		return nil, nil
	}

	plan.itemTitle = strings.TrimSpace(option.itemTitle)
	isOffline := option.isDryRun && option.isVaultChosen && plan.itemTitle != ""
	var vaults []cliprotocol.Vault
	switch {
	case isOffline:
		plan.vault = cliprotocol.Vault{Name: option.vaultName}
	case option.isVaultChosen:
		if plan.vault, err = findBackendVault(ctx, access, option.vaultName); err != nil {
			return nil, err
		}
	case isInteractive:
		if plan.vault, err = chooseDotenvVault(ctx, access); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("pass --vault and --item, or run hisar import in a terminal to choose them")
	}
	if !isOffline && !apitypes.CanWriteItems(plan.vault.Role) {
		return nil, fmt.Errorf("you can only read %q; choose a vault you can edit", plan.vault.Name)
	}
	if plan.itemTitle == "" {
		if !isInteractive {
			return nil, errors.New("pass --item with the name of the item to store the values in")
		}
		if plan.itemTitle, err = promptRequired("Item name", buildDefaultItemTitle(path)); err != nil {
			return nil, err
		}
		plan.itemTitle = strings.TrimSpace(plan.itemTitle)
	}
	vaultSegment := plan.vault.Name
	if !isOffline {
		if vaults, err = access.listVaults(ctx); err != nil {
			return nil, err
		}
		vaultSegment = buildReferenceVaultSegment(vaults, plan.vault)
		if plan.existingItemID, err = findDotenvItem(ctx, access, plan.vault, plan.itemTitle); err != nil {
			return nil, err
		}
	}
	for _, variable := range plan.storedVariables {
		plan.referencesByName[variable.Name] = secretreference.Reference{Vault: vaultSegment, Item: plan.itemTitle, Field: variable.Name}.String()
	}
	return plan, nil
}

func printDotenvPlan(plan *dotenvPlan) {
	action := "a new item"
	if plan.existingItemID != "" {
		action = "the existing item"
	}
	fmt.Fprintf(standardError(), "Store %s in %s › %s (%s), as concealed fields:\n", formatValueCount(len(plan.storedVariables)), plan.vault.Name, plan.itemTitle, action)
	for _, variable := range plan.storedVariables {
		note := ""
		if variable.OccurrenceCount > 1 && variable.HasOtherValue {
			note = fmt.Sprintf(" (set %d times; the last value is stored, as hisar run reads it)", variable.OccurrenceCount)
		}
		fmt.Fprintf(standardError(), "  %s%s\n", variable.Name, note)
	}
	fmt.Fprintf(standardError(), "Then rewrite %s:\n", plan.path)
	for _, variable := range plan.storedVariables {
		fmt.Fprintf(standardError(), "  %s=%s\n", variable.Name, plan.referencesByName[variable.Name])
	}
	if len(plan.keptNames) > 0 {
		fmt.Fprintf(standardError(), "Kept as they are: %s\n", strings.Join(plan.keptNames, ", "))
	}
}

func setDotenvValue(vaultItem *item.Item, name, value string) {
	if builtInField, isBuiltIn := secretreference.GetBuiltInField(name); isBuiltIn {
		switch builtInField {
		case secretreference.FieldPassword:
			vaultItem.Details.Password = value
		case secretreference.FieldUsername:
			vaultItem.Overview.Username = value
		case secretreference.FieldNotes:
			vaultItem.Details.Notes = value
		case secretreference.FieldURL:
			if len(vaultItem.Overview.URLs) == 0 {
				vaultItem.Overview.URLs = []string{value}
			} else {
				vaultItem.Overview.URLs[0] = value
			}
		}
		return
	}
	for index := range vaultItem.Details.Fields {
		if strings.EqualFold(vaultItem.Details.Fields[index].Name, name) {
			vaultItem.Details.Fields[index].Value, vaultItem.Details.Fields[index].Concealed = value, true
			return
		}
	}
	vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: name, Value: value, Concealed: true})
}

func storeDotenvItem(ctx context.Context, access backend, plan *dotenvPlan) error {
	vaultItem := &item.Item{Overview: item.Overview{Title: plan.itemTitle}}
	if plan.existingItemID != "" {
		existing, err := access.getItem(ctx, plan.vault.ID, plan.existingItemID)
		if err != nil {
			return err
		}
		vaultItem = existing
	}
	for _, variable := range plan.storedVariables {
		setDotenvValue(vaultItem, variable.Name, variable.Value)
	}
	for _, variable := range plan.storedVariables {
		value, _, err := secretreference.ReadField(vaultItem, variable.Name, time.Now())
		if err != nil || value != variable.Value {
			return fmt.Errorf("%s: %s can't be read back through its secret reference; nothing was stored", plan.itemTitle, variable.Name)
		}
	}
	return access.saveItem(ctx, plan.vault.ID, vaultItem)
}

// rewriteDotenvFile replaces the file in one rename, keeping its mode; no copy of the old content is kept.
func rewriteDotenvFile(path, content string, source dotenvSource) error {
	if err := checkDotenvSource(path, source); err != nil {
		return err
	}
	targetPath := source.targetPath
	temporaryFile, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".hisar-*")
	if err != nil {
		return err
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporaryFile.WriteString(content); err != nil {
		temporaryFile.Close()
		return err
	}
	if err := temporaryFile.Chmod(source.info.Mode().Perm()); err != nil {
		temporaryFile.Close()
		return err
	}
	if err := temporaryFile.Sync(); err != nil {
		temporaryFile.Close()
		return err
	}
	if err := temporaryFile.Close(); err != nil {
		return err
	}
	if err := checkDotenvSource(path, source); err != nil {
		return err
	}
	return os.Rename(temporaryPath, targetPath)
}

func formatValueCount(count int) string {
	if count == 1 {
		return "1 value"
	}
	return fmt.Sprintf("%d values", count)
}

func formatVariableCount(count int) string {
	if count == 1 {
		return "1 variable with a value"
	}
	return fmt.Sprintf("%d variables with a value", count)
}

func quoteShellWord(word string) string {
	if word != "" && strings.IndexFunc(word, func(character rune) bool {
		return !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("-_./~+=:@%", character))
	}) < 0 {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

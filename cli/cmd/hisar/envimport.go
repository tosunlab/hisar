package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

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
	layout           *dotenvLayout
	storedVariables  []dotenvVariable
	keptNames        []string
	vault            cliprotocol.Vault
	itemTitle        string
	existingItemID   string
	referencesByName map[string]string
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

func listStorableVariables(variables []dotenvVariable) (storable []dotenvVariable, skippedDescriptions []string) {
	for _, variable := range variables {
		switch {
		case variable.value == "":
			skippedDescriptions = append(skippedDescriptions, variable.name+" (empty)")
		case secretreference.IsReference(variable.value):
			skippedDescriptions = append(skippedDescriptions, variable.name+" (already a secret reference)")
		default:
			storable = append(storable, variable)
		}
	}
	return storable, skippedDescriptions
}

func chooseDotenvVariable(storable []dotenvVariable, chosenKeys []string) (map[string]bool, error) {
	chosenByName := map[string]bool{}
	if chosenKeys != nil {
		for _, key := range chosenKeys {
			if !slices.ContainsFunc(storable, func(variable dotenvVariable) bool { return variable.name == key }) {
				return nil, fmt.Errorf("--keys names %s, which is not a variable with a value in the file", key)
			}
			chosenByName[key] = true
		}
		return chosenByName, nil
	}
	for _, variable := range storable {
		chosenByName[variable.name] = variable.isSecretLooking
	}
	return chosenByName, nil
}

func printDotenvChoice(storable []dotenvVariable, chosenByName map[string]bool) {
	nameWidth := 0
	for _, variable := range storable {
		nameWidth = max(nameWidth, len(variable.name))
	}
	for index, variable := range storable {
		mark, description := " ", "stays a plain value ("+variable.reason+")"
		if chosenByName[variable.name] {
			mark, description = "x", "store in Hisar"
		}
		fmt.Fprintf(os.Stderr, "  [%s] %2d  %-*s  %s\n", mark, index+1, nameWidth, variable.name, description)
	}
}

// toggleDotenvChoice lets the user switch variables by number until they press Return.
func toggleDotenvChoice(storable []dotenvVariable, chosenByName map[string]bool) error {
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
				fmt.Fprintf(os.Stderr, "%s is not a number from 1 to %d.\n", field, len(storable))
				continue
			}
			name := storable[number-1].name
			chosenByName[name] = !chosenByName[name]
		}
	}
}

func chooseDotenvVault(ctx context.Context, access vaultAccess) (cliprotocol.Vault, error) {
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
	fmt.Fprintln(os.Stderr, "Vault:")
	for index, vault := range writable {
		fmt.Fprintf(os.Stderr, "  %d) %s\n", index+1, vault.Name)
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
		fmt.Fprintf(os.Stderr, "Enter a number from 1 to %d.\n", len(writable))
	}
}

func buildReferenceVaultSegment(vaults []cliprotocol.Vault, vault cliprotocol.Vault) string {
	sameNameCount := 0
	for _, candidate := range vaults {
		if strings.EqualFold(candidate.Name, vault.Name) {
			sameNameCount++
		}
	}
	if sameNameCount > 1 {
		return vault.ID
	}
	return vault.Name
}

func findDotenvItem(ctx context.Context, access vaultAccess, vault cliprotocol.Vault, title string) (string, error) {
	items, err := access.listItems(ctx, vault.ID)
	if err != nil {
		return "", err
	}
	matchIDs := []string{}
	for _, vaultItem := range items {
		if strings.EqualFold(vaultItem.Overview.Title, title) {
			matchIDs = append(matchIDs, vaultItem.ID)
		}
	}
	if len(matchIDs) > 1 {
		return "", fmt.Errorf("%d items in %s are titled %q; choose another item name", len(matchIDs), vault.Name, title)
	}
	if len(matchIDs) == 1 {
		return matchIDs[0], nil
	}
	return "", nil
}

func importDotenv(ctx context.Context, access vaultAccess, path string, option dotenvImportOption) error {
	plan, err := planDotenvImport(ctx, access, path, option)
	if err != nil || plan == nil {
		return err
	}
	printDotenvPlan(plan)
	if option.isDryRun {
		fmt.Fprintf(os.Stderr, "Dry run: nothing was stored and %s was not changed.\n", path)
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
	if err := storeDotenvItem(ctx, access, plan); err != nil {
		return err
	}
	if err := rewriteDotenvFile(plan.path, plan.layout.render(plan.referencesByName)); err != nil {
		return fmt.Errorf("the values are stored in %s › %s, but %s could not be rewritten: %w", plan.vault.Name, plan.itemTitle, path, err)
	}
	fmt.Fprintf(os.Stderr, "Stored %s in %s › %s and rewrote %s with secret references.\n", formatValueCount(len(plan.storedVariables)), plan.vault.Name, plan.itemTitle, path)
	fmt.Fprintf(os.Stderr, "Run a command with them:\n  hisar run --env-file %s -- <command>\n", quoteShellWord(path))
	return nil
}

func planDotenvImport(ctx context.Context, access vaultAccess, path string, option dotenvImportOption) (*dotenvPlan, error) {
	content, err := readImportFile(path)
	if err != nil {
		return nil, err
	}
	layout, err := parseDotenvLayout(path, string(content))
	clear(content)
	if err != nil {
		return nil, err
	}
	storable, skippedDescriptions := listStorableVariables(layout.listVariables())
	if len(storable) == 0 {
		fmt.Fprintf(os.Stderr, "%s has no values to store; it was not changed.\n", path)
		return nil, nil
	}
	chosenByName, err := chooseDotenvVariable(storable, option.chosenKeys)
	if err != nil {
		return nil, err
	}
	isInteractive := isInteractiveTerminal()
	if isInteractive && option.chosenKeys == nil {
		fmt.Fprintf(os.Stderr, "Found %s in %s:\n", formatVariableCount(len(storable)), path)
		if err := toggleDotenvChoice(storable, chosenByName); err != nil {
			return nil, err
		}
	}
	plan := &dotenvPlan{path: path, layout: layout, referencesByName: map[string]string{}}
	for _, variable := range storable {
		if !chosenByName[variable.name] {
			plan.keptNames = append(plan.keptNames, variable.name)
			continue
		}
		if err := checkStorableName(variable.name); err != nil {
			return nil, err
		}
		plan.storedVariables = append(plan.storedVariables, variable)
	}
	for _, description := range skippedDescriptions {
		plan.keptNames = append(plan.keptNames, description)
	}
	if len(plan.storedVariables) == 0 {
		fmt.Fprintf(os.Stderr, "No variables chosen; %s was not changed.\n", path)
		return nil, nil
	}

	plan.itemTitle = strings.TrimSpace(option.itemTitle)
	isOffline := option.isDryRun && option.isVaultChosen && plan.itemTitle != ""
	var vaults []cliprotocol.Vault
	switch {
	case isOffline:
		plan.vault = cliprotocol.Vault{Name: option.vaultName}
	case option.isVaultChosen:
		if plan.vault, err = findAccessVault(ctx, access, option.vaultName); err != nil {
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
		plan.referencesByName[variable.name] = secretreference.Reference{Vault: vaultSegment, Item: plan.itemTitle, Field: variable.name}.String()
	}
	return plan, nil
}

func printDotenvPlan(plan *dotenvPlan) {
	action := "a new item"
	if plan.existingItemID != "" {
		action = "the existing item"
	}
	fmt.Fprintf(os.Stderr, "Store %s in %s › %s (%s), as concealed fields:\n", formatValueCount(len(plan.storedVariables)), plan.vault.Name, plan.itemTitle, action)
	for _, variable := range plan.storedVariables {
		note := ""
		if variable.occurrenceCount > 1 && variable.hasOtherValue {
			note = fmt.Sprintf(" (set %d times; the last value is stored, as hisar run reads it)", variable.occurrenceCount)
		}
		fmt.Fprintf(os.Stderr, "  %s%s\n", variable.name, note)
	}
	fmt.Fprintf(os.Stderr, "Then rewrite %s:\n", plan.path)
	for _, variable := range plan.storedVariables {
		fmt.Fprintf(os.Stderr, "  %s=%s\n", variable.name, plan.referencesByName[variable.name])
	}
	if len(plan.keptNames) > 0 {
		fmt.Fprintf(os.Stderr, "Kept as they are: %s\n", strings.Join(plan.keptNames, ", "))
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

func storeDotenvItem(ctx context.Context, access vaultAccess, plan *dotenvPlan) error {
	vaultItem := &item.Item{Overview: item.Overview{Title: plan.itemTitle}}
	if plan.existingItemID != "" {
		existing, err := access.getItem(ctx, plan.vault.ID, plan.existingItemID)
		if err != nil {
			return err
		}
		vaultItem = existing
	}
	for _, variable := range plan.storedVariables {
		setDotenvValue(vaultItem, variable.name, variable.value)
	}
	for _, variable := range plan.storedVariables {
		value, _, err := secretreference.ReadField(vaultItem, variable.name, time.Now())
		if err != nil || value != variable.value {
			return fmt.Errorf("%s: %s can't be read back through its secret reference; nothing was stored", plan.itemTitle, variable.name)
		}
	}
	return access.saveItem(ctx, plan.vault.ID, vaultItem)
}

// rewriteDotenvFile replaces the file in one rename, keeping its mode; no copy of the old content is kept.
func rewriteDotenvFile(path, content string) error {
	targetPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	fileInfo, err := os.Stat(targetPath)
	if err != nil {
		return err
	}
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
	if err := temporaryFile.Chmod(fileInfo.Mode().Perm()); err != nil {
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

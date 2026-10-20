package dotenv

import (
	"strings"

	"github.com/tryhisar/hisar/core/secretreference"
)

// ReadFiles reads the files in order, as hisar run --env-file does.
func ReadFiles(paths []string) ([]Variable, error) {
	var variables []Variable
	for _, path := range paths {
		variablesInFile, err := ReadFile(path)
		if err != nil {
			return nil, err
		}
		variables = append(variables, variablesInFile...)
	}
	return variables, nil
}

// BuildEnvironment applies the files' variables in order over environ; a real variable wins over the files unless
// its value is a hisar:// reference.
func BuildEnvironment(environ []string, fileVariables []Variable) []Variable {
	var variables []Variable
	indexByName := map[string]int{}
	for _, entry := range environ {
		name, value, hasEquals := strings.Cut(entry, "=")
		if !hasEquals || name == "" {
			continue
		}
		if index, isKnown := indexByName[name]; isKnown {
			variables[index].Value = value
			continue
		}
		indexByName[name] = len(variables)
		variables = append(variables, Variable{Name: name, Value: value})
	}
	isFromFile := map[string]bool{}
	for _, fileVariable := range fileVariables {
		index, isKnown := indexByName[fileVariable.Name]
		if !isKnown {
			indexByName[fileVariable.Name] = len(variables)
			variables = append(variables, fileVariable)
			isFromFile[fileVariable.Name] = true
			continue
		}
		if isFromFile[fileVariable.Name] || secretreference.IsReference(variables[index].Value) {
			variables[index].Value = fileVariable.Value
			isFromFile[fileVariable.Name] = true
		}
	}
	return variables
}

// ListReferences returns each value that is a whole hisar:// reference once, in order.
func ListReferences(variables []Variable) []string {
	var references []string
	isListed := map[string]bool{}
	for _, variable := range variables {
		if secretreference.IsReference(variable.Value) && !isListed[variable.Value] {
			isListed[variable.Value] = true
			references = append(references, variable.Value)
		}
	}
	return references
}

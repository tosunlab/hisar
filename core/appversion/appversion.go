// Package appversion parses and compares app versions as semantic versions (MAJOR.MINOR.PATCH[-pre-release][+build]).
package appversion

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major      uint64
	Minor      uint64
	Patch      uint64
	PreRelease []string
}

func Parse(text string) (Version, error) {
	core, _, _ := strings.Cut(text, "+")
	core, preRelease, hasPreRelease := strings.Cut(core, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", text)
	}
	var numbers [3]uint64
	for index, part := range parts {
		number, err := parseNumber(part)
		if err != nil {
			return Version{}, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", text)
		}
		numbers[index] = number
	}
	version := Version{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]}
	if hasPreRelease {
		for identifier := range strings.SplitSeq(preRelease, ".") {
			if !isValidPreReleaseIdentifier(identifier) {
				return Version{}, fmt.Errorf("version %q has an invalid pre-release %q", text, preRelease)
			}
			version.PreRelease = append(version.PreRelease, identifier)
		}
	}
	return version, nil
}

func parseNumber(text string) (uint64, error) {
	if text == "" || (len(text) > 1 && text[0] == '0') || strings.TrimLeft(text, "0123456789") != "" {
		return 0, fmt.Errorf("%q is not a number", text)
	}
	return strconv.ParseUint(text, 10, 64)
}

func isValidPreReleaseIdentifier(identifier string) bool {
	if identifier == "" {
		return false
	}
	for _, character := range identifier {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character == '-') {
			return false
		}
	}
	if _, err := strconv.ParseUint(identifier, 10, 64); err == nil {
		return len(identifier) == 1 || identifier[0] != '0'
	}
	return true
}

func (version Version) IsPreRelease() bool { return len(version.PreRelease) > 0 }

func (version Version) String() string {
	text := fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)
	if version.IsPreRelease() {
		text += "-" + strings.Join(version.PreRelease, ".")
	}
	return text
}

// Compare follows semantic versioning precedence: a pre-release comes before its release, and build metadata is ignored.
func Compare(first, second Version) int {
	if result := cmp.Or(cmp.Compare(first.Major, second.Major), cmp.Compare(first.Minor, second.Minor), cmp.Compare(first.Patch, second.Patch)); result != 0 {
		return result
	}
	switch {
	case !first.IsPreRelease() && !second.IsPreRelease():
		return 0
	case !first.IsPreRelease():
		return 1
	case !second.IsPreRelease():
		return -1
	}
	for index := 0; index < min(len(first.PreRelease), len(second.PreRelease)); index++ {
		if result := compareIdentifier(first.PreRelease[index], second.PreRelease[index]); result != 0 {
			return result
		}
	}
	return cmp.Compare(len(first.PreRelease), len(second.PreRelease))
}

func compareIdentifier(first, second string) int {
	firstNumber, firstError := strconv.ParseUint(first, 10, 64)
	secondNumber, secondError := strconv.ParseUint(second, 10, 64)
	switch {
	case firstError == nil && secondError == nil:
		return cmp.Compare(firstNumber, secondNumber)
	case firstError == nil:
		return -1
	case secondError == nil:
		return 1
	}
	return strings.Compare(first, second)
}

// IsOlder reports whether current is below minimum; it fails when either is not a version.
func IsOlder(current, minimum string) (bool, error) {
	currentVersion, err := Parse(current)
	if err != nil {
		return false, err
	}
	minimumVersion, err := Parse(minimum)
	if err != nil {
		return false, err
	}
	return Compare(currentVersion, minimumVersion) < 0, nil
}

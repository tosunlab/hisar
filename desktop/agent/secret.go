package agent

import "fmt"

const (
	shownSecretCount       = 4
	maximumSecretNameCount = 200
)

func describeSecretAction(secretCount int, isRun bool) string {
	noun := "secrets"
	if secretCount == 1 {
		noun = "secret"
	}
	if isRun {
		return fmt.Sprintf("run a command with %d %s", secretCount, noun)
	}
	return fmt.Sprintf("read %d %s", secretCount, noun)
}

func limitSecretName(secretNames []string) ([]string, int) {
	shownCount := min(len(secretNames), shownSecretCount)
	shown := make([]string, 0, shownCount)
	for _, name := range secretNames[:shownCount] {
		shown = append(shown, truncateText(name, maximumSecretNameCount))
	}
	return shown, len(secretNames) - shownCount
}

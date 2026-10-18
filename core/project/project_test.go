package project

import (
	"regexp"
	"testing"
)

func TestGitHubRepositoryIsOwnerAndName(t *testing.T) {
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9._-]{1,100}$`).MatchString(GitHubRepository) {
		t.Errorf("GitHubRepository = %q is not owner/name", GitHubRepository)
	}
	if GitHubURL != "https://github.com/"+GitHubRepository {
		t.Errorf("GitHubURL = %q", GitHubURL)
	}
}

package main

import (
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

func startFavoritesApp(t *testing.T, favorites []cliprotocol.Favorite) *fakeApp {
	t.Helper()
	app := startFakeApp(t)
	app.mutex.Lock()
	app.answer = func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
		switch request.Type {
		case cliprotocol.TypeListFavorite:
			return cliprotocol.FavoriteListResult{Favorites: favorites}, nil
		case cliprotocol.TypeAddFavorite, cliprotocol.TypeRemoveFavorite:
			return struct{}{}, nil
		}
		return answerFakeVaultRequest(request)
	}
	app.mutex.Unlock()
	return app
}

func TestFavoriteCommandsThroughApp(t *testing.T) {
	app := startFavoritesApp(t, []cliprotocol.Favorite{{VaultID: "personal-id", ItemID: "github-id"}, {VaultID: "family-id", ItemID: "bank-id"}})

	if _, stderr, err := runCapturingOutput(t, false, "", "favorite", "github"); err != nil || !strings.Contains(stderr, `Added "GitHub" (Personal) to Favorites.`) {
		t.Fatalf("favorite: %v %s", err, stderr)
	}
	if added := app.findRequest(cliprotocol.TypeAddFavorite); len(added) != 1 || added[0].VaultID != "personal-id" || added[0].ItemID != "github-id" {
		t.Errorf("add requests: %+v", added)
	}
	if _, stderr, err := runCapturingOutput(t, false, "", "unfavorite", "Bank", "-v", "Family"); err != nil || !strings.Contains(stderr, `Removed "Bank" (Family) from Favorites.`) {
		t.Fatalf("unfavorite: %v %s", err, stderr)
	}
	if removed := app.findRequest(cliprotocol.TypeRemoveFavorite); len(removed) != 1 || removed[0].VaultID != "family-id" || removed[0].ItemID != "bank-id" {
		t.Errorf("remove requests: %+v", removed)
	}
	if _, _, err := runCapturingOutput(t, false, "", "favorite", "no such item"); err == nil || !strings.Contains(err.Error(), "no item matches") {
		t.Errorf("an unknown item: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "favorite"); err != errUsage {
		t.Errorf("no item: %v", err)
	}

	stdout, _, err := runCapturingOutput(t, false, "", "ls", "--favorites")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], "VAULT") || !strings.HasPrefix(lines[1], "Bank") || !strings.Contains(lines[1], "Family") ||
		!strings.HasPrefix(lines[2], "GitHub") || !strings.Contains(lines[2], "Personal") {
		t.Errorf("ls --favorites:\n%s", stdout)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "ls", "--favorites", "-v", "Personal")
	if err != nil || strings.Contains(stdout, "Bank") || !strings.Contains(stdout, "GitHub") {
		t.Errorf("ls --favorites -v Personal: %v\n%s", err, stdout)
	}
	if stdout, _, _ := runCapturingOutput(t, false, "", "ls"); strings.Contains(stdout, "VAULT") {
		t.Errorf("plain ls shows the favorites layout:\n%s", stdout)
	}
}

func TestFavoritesWithoutAny(t *testing.T) {
	startFavoritesApp(t, []cliprotocol.Favorite{})
	stdout, stderr, err := runCapturingOutput(t, false, "", "ls", "--favorites")
	if err != nil || stdout != "" || !strings.Contains(stderr, "No favorites yet. Add one with: hisar favorite <item>") {
		t.Errorf("no favorites: %v %q %q", err, stdout, stderr)
	}
}

func TestFavoritesNeedAnAppThatKeepsThem(t *testing.T) {
	app := startFavoritesApp(t, nil)
	app.mutex.Lock()
	app.hello.Features = []string{cliprotocol.FeatureSecureNotes, cliprotocol.FeatureCreditCards}
	app.mutex.Unlock()
	for _, args := range [][]string{{"favorite", "GitHub"}, {"ls", "--favorites"}} {
		if _, _, err := runCapturingOutput(t, false, "", args...); err == nil || !strings.Contains(err.Error(), "update the Hisar app, or run hisar with --standalone") {
			t.Errorf("%v with an older app: %v", args, err)
		}
	}
	if len(app.findRequest(cliprotocol.TypeAddFavorite)) != 0 {
		t.Error("an older app was asked to keep a favorite")
	}
}

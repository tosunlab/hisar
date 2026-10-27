package item

import (
	"encoding/json"
	"testing"
)

// The vectors are the Mac's JavaScript rule, which this replaced, run over each password. The one difference is the
// last row: the Mac counted UTF-16 code units, five emoji as ten, and the rule now counts characters as the session does.
var masterPasswordVectors = []struct {
	password   string
	level      string
	bits       int
	message    string
	isBlocking bool
}{
	{"", "", 0, "A few random words you can remember make a strong master password.", false},
	{"a", "weak", 5, "Use at least 10 characters.", true},
	{"hunter2", "weak", 36, "Use at least 10 characters.", true},
	{"abcdef", "weak", 0, "Use at least 10 characters.", true},
	{"aaaaaaaaaaaaaaaa", "weak", 26, "This password is too easy to guess. Add a few more words or characters.", true},
	{"123456789", "weak", 0, "Use at least 10 characters.", true},
	{"1234567890", "weak", 0, "This password is too easy to guess. Add a few more words or characters.", true},
	{"password", "weak", 14, "Use at least 10 characters.", true},
	{"password1", "weak", 23, "Use at least 10 characters.", true},
	{"password1234", "weak", 8, "This password is too easy to guess. Add a few more words or characters.", true},
	{"Password1234!", "weak", 27, "This password is too easy to guess. Add a few more words or characters.", true},
	{"PASSWORD1234", "weak", 8, "This password is too easy to guess. Add a few more words or characters.", true},
	{"qwertyuiop", "weak", 24, "This password is too easy to guess. Add a few more words or characters.", true},
	{"letmein2024", "weak", 17, "This password is too easy to guess. Add a few more words or characters.", true},
	{"iloveyou!!", "weak", 35, "You can use it, but another word or a few more characters make it much harder to guess.", false},
	{"welcome-admin", "weak", 0, "This password is too easy to guess. Add a few more words or characters.", true},
	{"Spring2024!", "weak", 32, "You can use it, but another word or a few more characters make it much harder to guess.", false},
	{"fallingwater99", "fair", 45, "A few random words you can remember make a strong master password.", false},
	{"winter is coming", "weak", 13, "This password is too easy to guess. Add a few more words or characters.", true},
	{"Mx9$Lp2!Qw7@", "good", 79, "A few random words you can remember make a strong master password.", false},
	{"k7#Vq2!mZp9@Lx4$Rt8w", "strong", 131, "A few random words you can remember make a strong master password.", false},
	{"abcdefghij", "weak", 0, "This password is too easy to guess. Add a few more words or characters.", true},
	{"zyxwvutsrq", "weak", 17, "This password is too easy to guess. Add a few more words or characters.", true},
	{"1928374650", "weak", 11, "This password is too easy to guess. Add a few more words or characters.", true},
	{"kx7Tq9wLm2", "good", 60, "A few random words you can remember make a strong master password.", false},
	{"kx7Tq9wLm2#", "good", 72, "A few random words you can remember make a strong master password.", false},
	{"tr0ub4dor&3", "good", 67, "A few random words you can remember make a strong master password.", false},
	{"netflix-family-pass", "weak", 13, "This password is too easy to guess. Add a few more words or characters.", true},
	{"orange cactus harbor lantern", "fair", 44, "A few random words you can remember make a strong master password.", false},
	{"correct-horse-battery-staple", "fair", 44, "A few random words you can remember make a strong master password.", false},
	{"correct-horse-battery-staple-orbit-lantern-mosaic-tundra", "good", 88, "A few random words you can remember make a strong master password.", false},
	{"Correct.Horse_Battery Staple", "fair", 44, "A few random words you can remember make a strong master password.", false},
	{"orange--cactus", "good", 74, "A few random words you can remember make a strong master password.", false},
	{"-orange-cactus", "good", 78, "A few random words you can remember make a strong master password.", false},
	{"orange-cactus-", "good", 78, "A few random words you can remember make a strong master password.", false},
	{"orange-cactus", "weak", 22, "This password is too easy to guess. Add a few more words or characters.", true},
	{"orangecactus", "fair", 53, "A few random words you can remember make a strong master password.", false},
	{"two words", "weak", 22, "Use at least 10 characters.", true},
	{"orange cactus harbor", "weak", 33, "You can use it, but another word or a few more characters make it much harder to guess.", false},
	{"çilek-armut-elma", "good", 86, "A few random words you can remember make a strong master password.", false},
	{"şifre1234567", "weak", 28, "You can use it, but another word or a few more characters make it much harder to guess.", false},
	{"ünïcödé passwörd", "good", 70, "A few random words you can remember make a strong master password.", false},
	{"пароль123456", "weak", 26, "This password is too easy to guess. Add a few more words or characters.", true},
	{"🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂", "strong", 101, "A few random words you can remember make a strong master password.", false},
	{"🙂🙃🙂🙃🙂🙃🙂🙃🙂🙃", "strong", 101, "A few random words you can remember make a strong master password.", false},
	{"日本語のパスワードです", "fair", 55, "A few random words you can remember make a strong master password.", false},
	{"a1b2c3d4e5f6", "good", 62, "A few random words you can remember make a strong master password.", false},
	{"ABCabcABCabc", "weak", 16, "This password is too easy to guess. Add a few more words or characters.", true},
	{"1999-12-31-xyz", "fair", 44, "A few random words you can remember make a strong master password.", false},
	{"Admin!Admin!", "fair", 57, "A few random words you can remember make a strong master password.", false},
	{"        tabbed", "fair", 41, "A few random words you can remember make a strong master password.", false},
	{"Ab1!Ab1!Ab1!Ab1!", "strong", 105, "A few random words you can remember make a strong master password.", false},
	{"qmz vbx kjw plt", "fair", 44, "A few random words you can remember make a strong master password.", false},
	{"9f8e7d6c5b4a", "good", 62, "A few random words you can remember make a strong master password.", false},
	{"🙂🙃🙂🙃🙂", "fair", 50, "Use at least 10 characters.", true},
}

func TestCheckMasterPasswordVectors(t *testing.T) {
	labels := map[string]string{"": "", "weak": "Weak", "fair": "Fair", "good": "Good", "strong": "Strong"}
	for _, vector := range masterPasswordVectors {
		want := MasterPasswordFeedback{Level: vector.level, Label: labels[vector.level], Bits: vector.bits, Message: vector.message, IsBlocking: vector.isBlocking}
		if got := CheckMasterPassword(vector.password); got != want {
			t.Errorf("CheckMasterPassword(%q) = %+v, want %+v", vector.password, got, want)
		}
		if got := RatePassword(vector.password); got != (PasswordStrength{Level: want.Level, Label: want.Label, Bits: want.Bits}) {
			t.Errorf("RatePassword(%q) = %+v", vector.password, got)
		}
	}
}

func TestMasterPasswordFeedbackJSON(t *testing.T) {
	encoded, err := json.Marshal(CheckMasterPassword("password1234"))
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"level":"weak","label":"Weak","bits":8,"message":"This password is too easy to guess. Add a few more words or characters.","isBlocking":true}`
	if string(encoded) != want {
		t.Fatalf("got %s, want %s", encoded, want)
	}
}

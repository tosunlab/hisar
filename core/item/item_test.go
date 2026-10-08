package item

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	cases := []GenOptions{
		DefaultGenOptions,
		{Length: 4, Digits: true, Symbols: true},
		{Length: 12},
		{Length: 64, Digits: true},
	}
	for _, o := range cases {
		seen := map[string]bool{}
		for range 50 {
			pw, err := Generate(o)
			if err != nil {
				t.Fatalf("%+v: %v", o, err)
			}
			if len(pw) != o.Length {
				t.Fatalf("%+v: length %d", o, len(pw))
			}
			if !strings.ContainsAny(pw, lower) || !strings.ContainsAny(pw, upper) {
				t.Errorf("%+v: missing letters in %q", o, pw)
			}
			if o.Digits != strings.ContainsAny(pw, digits) {
				t.Errorf("%+v: digits mismatch in %q", o, pw)
			}
			if o.Symbols != strings.ContainsAny(pw, symbols) {
				t.Errorf("%+v: symbols mismatch in %q", o, pw)
			}
			if strings.ContainsAny(pw, "lIO01") {
				t.Errorf("%+v: ambiguous character in %q", o, pw)
			}
			seen[pw] = true
		}
		if o.Length >= 12 && len(seen) != 50 {
			t.Errorf("%+v: duplicate passwords generated", o)
		}
	}
}

func TestGenerateRejectsBadLength(t *testing.T) {
	for _, o := range []GenOptions{{Length: 3, Digits: true, Symbols: true}, {Length: 1}, {Length: 257}} {
		if _, err := Generate(o); err == nil {
			t.Errorf("%+v accepted", o)
		}
	}
}

func TestGenerateDistribution(t *testing.T) {
	counts := map[byte]int{}
	const n = 2000
	for range n {
		pw, _ := Generate(GenOptions{Length: 32})
		for i := range len(pw) {
			counts[pw[i]]++
		}
	}
	alphabet := len(lower) + len(upper)
	want := n * 32 / alphabet
	for c, got := range counts {
		if got < want/2 || got > want*2 {
			t.Errorf("char %q appeared %d times, expected about %d", c, got, want)
		}
	}
	if len(counts) != alphabet {
		t.Errorf("used %d distinct chars, want %d", len(counts), alphabet)
	}
}

package credential

import (
	"strings"
	"testing"
)

func TestNewRecoveryCodesAreDistinctAndWellFormed(t *testing.T) {
	codes, err := NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating codes: %v", err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("got %d codes, want %d", len(codes), RecoveryCodeCount)
	}

	seen := map[string]bool{}
	for _, code := range codes {
		if seen[code] {
			t.Fatalf("duplicate code %q in one set", code)
		}
		seen[code] = true

		if want := "XXXXX-XXXXX-XXXXX"; len(code) != len(want) {
			t.Errorf("code %q is %d characters, want %d", code, len(code), len(want))
		}
		for _, r := range NormalizeRecoveryCode(code) {
			if !strings.ContainsRune(recoveryAlphabet, r) {
				t.Errorf("code %q contains %q, which is outside the alphabet", code, r)
			}
		}
		// The excluded characters are the whole point of the alphabet.
		for _, bad := range []string{"I", "L", "O", "U"} {
			if strings.Contains(code, bad) {
				t.Errorf("code %q contains %q, which reads as another character", code, bad)
			}
		}
	}
}

// A code copied off paper arrives in whatever shape the person typed it.
func TestNormalizeRecoveryCodeAcceptsHowPeopleType(t *testing.T) {
	canonical := NormalizeRecoveryCode("7KQ4N-2WX8F-MP3RT")

	for _, variant := range []string{
		"7KQ4N-2WX8F-MP3RT",
		"7kq4n-2wx8f-mp3rt",
		"7KQ4N2WX8FMP3RT",
		"  7KQ4N 2WX8F MP3RT  ",
		"7KQ4N--2WX8F--MP3RT",
	} {
		if got := NormalizeRecoveryCode(variant); got != canonical {
			t.Errorf("NormalizeRecoveryCode(%q) = %q, want %q", variant, got, canonical)
		}
	}
}

func TestHashRecoveryCodeIgnoresFormatting(t *testing.T) {
	a := HashRecoveryCode("7KQ4N-2WX8F-MP3RT")
	b := HashRecoveryCode("7kq4n2wx8fmp3rt")
	if string(a) != string(b) {
		t.Fatal("the same code in two formats hashed differently")
	}

	other := HashRecoveryCode("7KQ4N-2WX8F-MP3RV")
	if string(a) == string(other) {
		t.Fatal("two different codes hashed the same")
	}
}

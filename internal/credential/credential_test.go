package credential

import (
	"strings"
	"testing"
)

func TestMintProducesDistinctUnguessableCredentials(t *testing.T) {
	seen := map[string]bool{}
	publicIDs := map[string]bool{}

	for range 500 {
		tok, err := Mint(KindMachine)
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		if seen[tok.Secret] {
			t.Fatal("mint produced a duplicate credential")
		}
		if publicIDs[tok.PublicID] {
			t.Fatal("mint produced a duplicate public id")
		}
		seen[tok.Secret] = true
		publicIDs[tok.PublicID] = true

		if !strings.HasPrefix(tok.Secret, "vlt_") {
			t.Fatalf("credential lacks its kind prefix: %q", tok.Secret[:8])
		}
	}
}

// The public half is used for lookup and appears in listings, so it must not be
// derivable from the secret half or vice versa.
func TestPublicIDIsIndependentOfSecret(t *testing.T) {
	tok, err := Mint(KindMachine)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	parts := strings.Split(tok.Secret, "_")
	if len(parts) != 3 {
		t.Fatalf("unexpected credential shape")
	}
	if parts[1] != tok.PublicID {
		t.Fatal("public id does not match the credential")
	}
	if strings.Contains(parts[2], tok.PublicID) {
		t.Fatal("the secret half contains the public half")
	}
}

// Regression test. An encoding whose alphabet contains the field delimiter
// produces credentials that split into the wrong number of pieces for some
// fraction of the random values, a bug that shows up intermittently and
// authenticates nobody, or worse, truncates the compared secret. The encoding
// must never emit an underscore, so mint a large sample and check the shape.
func TestCredentialsAlwaysHaveExactlyThreeFields(t *testing.T) {
	for _, kind := range []Kind{KindMachine, KindRuntime, KindSession} {
		for range 2000 {
			tok, err := Mint(kind)
			if err != nil {
				t.Fatalf("mint: %v", err)
			}
			if got := strings.Count(tok.Secret, "_"); got != 2 {
				t.Fatalf("credential %q has %d delimiters, want 2", tok.Secret, got)
			}
			gotKind, gotPublic, err := Parse(tok.Secret)
			if err != nil {
				t.Fatalf("minted credential failed to parse: %v", err)
			}
			if gotKind != kind || gotPublic != tok.PublicID {
				t.Fatalf("round trip mismatch for %q", tok.Secret)
			}
		}
	}
}

func TestVerify(t *testing.T) {
	tok, err := Mint(KindRuntime)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	if !Verify(tok.Secret, tok.Hash) {
		t.Fatal("a freshly minted credential failed to verify")
	}
	if Verify(tok.Secret+"x", tok.Hash) {
		t.Fatal("a modified credential verified")
	}
	if Verify("", tok.Hash) {
		t.Fatal("an empty credential verified")
	}

	other, _ := Mint(KindRuntime)
	if Verify(other.Secret, tok.Hash) {
		t.Fatal("a different credential verified")
	}
}

// The stored verifier must not contain the credential itself in any form.
func TestHashDoesNotContainCredential(t *testing.T) {
	tok, err := Mint(KindMachine)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if strings.Contains(string(tok.Hash), tok.Secret) {
		t.Fatal("the stored verifier contains the credential")
	}
	if len(tok.Hash) != 32 {
		t.Fatalf("expected a 32-byte verifier, got %d", len(tok.Hash))
	}
}

func TestParse(t *testing.T) {
	tok, _ := Mint(KindMachine)

	kind, publicID, err := Parse(tok.Secret)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if kind != KindMachine || publicID != tok.PublicID {
		t.Fatalf("parse returned %s/%s", kind, publicID)
	}

	malformed := []string{
		"", "vlt", "vlt_abc", "vlt_abc_def_ghi",
		"xxx_" + tok.PublicID + "_short",
		"vlt_short_" + strings.Split(tok.Secret, "_")[2],
		"vlt_" + tok.PublicID + "_" + strings.Split(tok.Secret, "_")[2][:10],
		"Bearer " + tok.Secret,
	}
	for _, m := range malformed {
		if _, _, err := Parse(m); err == nil {
			t.Fatalf("parse accepted %q", m)
		}
	}
}

// The display form is what a listing screen renders. It must never be usable.
func TestDisplayIsNotUsable(t *testing.T) {
	tok, _ := Mint(KindMachine)
	shown := Display(KindMachine, tok.PublicID)

	if Verify(shown, tok.Hash) {
		t.Fatal("the display form authenticated")
	}
	if strings.Contains(tok.Secret, shown) {
		t.Fatal("the display form is a prefix of the real credential")
	}
	if !strings.Contains(shown, tok.PublicID) {
		t.Fatal("the display form should still identify the credential")
	}
}

func TestPasswordHashing(t *testing.T) {
	const password = "correct horse battery staple"

	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if strings.Contains(encoded, password) {
		t.Fatal("the stored hash contains the password")
	}
	if !strings.HasPrefix(encoded, "$argon2id$") {
		t.Fatalf("unexpected hash format: %q", encoded)
	}

	ok, err := VerifyPassword(password, encoded)
	if err != nil || !ok {
		t.Fatalf("correct password did not verify: ok=%v err=%v", ok, err)
	}

	ok, err = VerifyPassword("wrong password", encoded)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Fatal("an incorrect password verified")
	}
}

// Two accounts with the same password must not produce the same stored hash, or
// a database reader could tell who shares a password with whom.
func TestPasswordHashesAreSalted(t *testing.T) {
	a, _ := HashPassword("same password")
	b, _ := HashPassword("same password")

	if a == b {
		t.Fatal("identical passwords produced identical hashes")
	}
}

func TestVerifyPasswordRejectsCorruptHash(t *testing.T) {
	corrupt := []string{
		"", "not-a-hash", "$argon2id$", "$argon2i$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=0,t=0,p=0$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=2$!!!$aGFzaA",
	}
	for _, c := range corrupt {
		if _, err := VerifyPassword("anything", c); err == nil {
			t.Fatalf("accepted a corrupt hash: %q", c)
		}
	}
}

func TestNeedsRehash(t *testing.T) {
	current, _ := HashPassword("password")
	if NeedsRehash(current) {
		t.Fatal("a hash at current parameters was flagged for upgrade")
	}

	weak := "$argon2id$v=19$m=4096,t=1,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGE"
	if !NeedsRehash(weak) {
		t.Fatal("a hash at weaker parameters was not flagged for upgrade")
	}
	if !NeedsRehash("garbage") {
		t.Fatal("an unparseable hash should be replaced")
	}
}

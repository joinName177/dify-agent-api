package credentials

import (
	"strings"
	"testing"
)

func TestPasswordHasherVerifiesOnlyMatchingPassword(t *testing.T) {
	hasher := PasswordHasher{}
	hash, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if !hasher.Verify("correct horse battery staple", hash) {
		t.Fatal("matching password was rejected")
	}
	if hasher.Verify("wrong password", hash) {
		t.Fatal("wrong password was accepted")
	}
}

func TestPasswordHasherUsesUniqueSalt(t *testing.T) {
	hasher := PasswordHasher{}
	first, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash first password: %v", err)
	}
	second, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash second password: %v", err)
	}
	if first == second {
		t.Fatal("equal passwords produced equal hashes")
	}
}

func TestPasswordHasherRejectsUnboundedOrNonCanonicalParameters(t *testing.T) {
	hasher := PasswordHasher{}
	hash, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	for _, corrupted := range []string{
		strings.Replace(hash, "m=19456", "m=4294967295", 1),
		strings.Replace(hash, "t=2", "t=4294967295", 1),
		hash + "AAAA",
	} {
		if hasher.Verify("correct horse battery staple", corrupted) {
			t.Fatal("accepted non-canonical password hash parameters")
		}
	}
}

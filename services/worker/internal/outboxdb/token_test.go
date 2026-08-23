package outboxdb

import (
	"encoding/base64"
	"encoding/hex"
	"testing"
)

const goldenClaimTokenWire = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

func TestClaimTokenCandidateDigestGolden(t *testing.T) {
	t.Parallel()

	digest, ok := claimTokenCandidateDigest(goldenClaimTokenWire)
	if !ok {
		t.Fatal("canonical Golden claim token was rejected")
	}
	if got, want := hex.EncodeToString(digest[:]), "0fced3787dc44e7855171187da0812df307108fb766c97f6082824715b310994"; got != want {
		t.Fatalf("candidate digest = %s, want Golden %s", got, want)
	}
}

func TestClaimTokenCandidateDigestRejectsEveryNonCanonicalWireShape(t *testing.T) {
	t.Parallel()

	encoded31 := base64.RawURLEncoding.EncodeToString(make([]byte, 31))
	encoded33 := base64.RawURLEncoding.EncodeToString(make([]byte, 33))
	tests := map[string]string{
		"padded":                     goldenClaimTokenWire + "=",
		"42 characters":              goldenClaimTokenWire[:42],
		"44 characters":              goldenClaimTokenWire + "A",
		"space":                      " " + goldenClaimTokenWire[1:],
		"carriage return":            goldenClaimTokenWire[:20] + "\r" + goldenClaimTokenWire[21:],
		"line feed":                  goldenClaimTokenWire[:20] + "\n" + goldenClaimTokenWire[21:],
		"standard alphabet plus":     "+" + goldenClaimTokenWire[1:],
		"standard alphabet slash":    "/" + goldenClaimTokenWire[1:],
		"non-zero trailing bits":     goldenClaimTokenWire[:42] + "9",
		"31 decoded bytes":           encoded31,
		"33 decoded bytes":           encoded33,
		"non-ASCII replacement byte": goldenClaimTokenWire[:20] + "é" + goldenClaimTokenWire[22:],
	}

	for name, wire := range tests {
		name, wire := name, wire
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, ok := claimTokenCandidateDigest(wire); ok {
				t.Fatalf("non-canonical claim token %q was accepted", name)
			}
		})
	}
}

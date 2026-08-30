package grantcredential

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCanonicalTranscriptUsesJCSStringEscaping(t *testing.T) {
	t.Parallel()

	fixture := loadFixture(t)
	grant := presentedFixture(t, fixture.Valid.Grant)
	grant.PolicyVersion = "quote\" slash\\ <>& \u2028 \u2029 雪 😀"
	transcript, err := canonicalTranscript(grant)
	if err != nil {
		t.Fatalf("canonicalTranscript() error = %v", err)
	}
	want := []byte("\"policyVersion\":\"quote\\\" slash\\\\ <>& \u2028 \u2029 雪 😀\"")
	if !bytes.Contains(transcript, want) {
		t.Fatalf("transcript does not use the frozen JCS string escaping")
	}
	for _, forbidden := range [][]byte{[]byte(`\u003c`), []byte(`\u003e`), []byte(`\u0026`), []byte(`\u2028`), []byte(`\u2029`)} {
		if bytes.Contains(transcript, forbidden) {
			t.Fatalf("transcript contains non-JCS escape %q", forbidden)
		}
	}
}

func TestCanonicalTranscriptRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	fixture := loadFixture(t)
	grant := presentedFixture(t, fixture.Valid.Grant)
	grant.Scope.WorkspacePathPrefixes[0] = string([]byte{0xff})
	assertCredentialError(t, ErrorInvalidInput, func() error {
		_, err := canonicalTranscript(grant)
		return err
	})
}

func FuzzJSONStringRoundTrip(f *testing.F) {
	for _, seed := range []string{"plain", `quote"slash\\`, "<>&", "\u2028\u2029", "雪", "😀", "line\nfeed"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			t.Skip()
		}
		var encoded bytes.Buffer
		writeJSONString(&encoded, value)
		var decoded string
		if err := json.Unmarshal(encoded.Bytes(), &decoded); err != nil {
			t.Fatalf("generated JSON string is invalid: %v", err)
		}
		if decoded != value {
			t.Fatalf("JSON string round trip changed the value")
		}
	})
}

package grantcredential

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/core/internal/capability/grantpolicy"
)

type fixtureActor struct {
	ActorID   string `json:"actorId"`
	ActorType int32  `json:"actorType"`
}

type fixtureScope struct {
	ChannelIDs            []string `json:"channelIds"`
	DMIDs                 []string `json:"dmIds"`
	EventIDs              []string `json:"eventIds"`
	ThreadIDs             []string `json:"threadIds"`
	FileIDs               []string `json:"fileIds"`
	WorkspaceBindingIDs   []string `json:"workspaceBindingIds"`
	WorkspacePathPrefixes []string `json:"workspacePathPrefixes"`
	ToolIDs               []string `json:"toolIds"`
}

type fixtureGrant struct {
	CapabilityGrantID       string       `json:"capabilityGrantId"`
	TenantID                string       `json:"tenantId"`
	TaskID                  string       `json:"taskId"`
	RunID                   string       `json:"runId"`
	Grantee                 fixtureActor `json:"grantee"`
	Initiator               fixtureActor `json:"initiator"`
	Capabilities            []int32      `json:"capabilities"`
	ResourceScope           fixtureScope `json:"resourceScope"`
	IssuedAt                string       `json:"issuedAt"`
	ExpiresAt               string       `json:"expiresAt"`
	NonceHex                string       `json:"nonceHex"`
	PolicyVersion           string       `json:"policyVersion"`
	SignatureHex            string       `json:"signatureHex"`
	ExecutionDeviceID       string       `json:"executionDeviceId"`
	SignatureProfile        string       `json:"signatureProfile"`
	SigningKeyID            string       `json:"signingKeyId"`
	SignedProjectionVersion uint32       `json:"signedProjectionVersion"`
}

type fixtureAudience struct {
	TenantID          string       `json:"tenantId"`
	TaskID            string       `json:"taskId"`
	RunID             string       `json:"runId"`
	Grantee           fixtureActor `json:"grantee"`
	ExecutionDeviceID string       `json:"executionDeviceId"`
}

type fixtureFile struct {
	Valid struct {
		Grant                    fixtureGrant    `json:"grant"`
		PublicKeyRawHex          string          `json:"publicKeyRawHex"`
		ExpectedTranscriptSHA256 string          `json:"expectedTranscriptSha256"`
		ExpectedAudience         fixtureAudience `json:"expectedAudience"`
	} `json:"valid"`
}

func TestIssueAndVerifyMatchPublicFixture(t *testing.T) {
	t.Parallel()

	fixture := loadFixture(t)
	issued, verifier, expected := issueFixture(t, fixture)
	fixtureGrant := presentedFixture(t, fixture.Valid.Grant)

	if got, want := hex.EncodeToString(issued.Signature), fixture.Valid.Grant.SignatureHex; got != want {
		t.Fatal("issued signature does not match public fixture")
	}
	if !reflect.DeepEqual(issued, fixtureGrant) {
		t.Fatalf("issued Grant does not match public fixture")
	}
	transcript, err := canonicalTranscript(issued)
	if err != nil {
		t.Fatalf("canonicalTranscript() error = %v", err)
	}
	if got := sha256.Sum256(transcript); hex.EncodeToString(got[:]) != fixture.Valid.ExpectedTranscriptSHA256 {
		t.Fatalf("canonical transcript hash does not match public fixture")
	}

	verified, err := verifier.VerifyIntegrity(issued.IssuedAt, issued, expected)
	if err != nil {
		t.Fatalf("VerifyIntegrity() error = %v", err)
	}
	if verified.CapabilityGrantID() != issued.CapabilityGrantID ||
		verified.TenantID() != issued.TenantID || verified.TaskID() != issued.TaskID ||
		verified.RunID() != issued.RunID || verified.Grantee() != issued.Grantee ||
		verified.Initiator() != issued.Initiator ||
		verified.ExecutionDeviceID() != issued.ExecutionDeviceID ||
		verified.PolicyVersion() != issued.PolicyVersion ||
		!verified.IssuedAt().Equal(issued.IssuedAt) || !verified.ExpiresAt().Equal(issued.ExpiresAt) ||
		!reflect.DeepEqual(verified.Capabilities(), issued.Capabilities) ||
		!reflect.DeepEqual(verified.Scope(), issued.Scope) {
		t.Fatalf("verified facts differ from signed Grant")
	}
}

func TestVerifyIntegrityRejectsEverySignedMutation(t *testing.T) {
	t.Parallel()

	fixture := loadFixture(t)
	grant, verifier, expected := issueFixture(t, fixture)
	mutations := []struct {
		name   string
		mutate func(*PresentedGrant)
	}{
		{"grant-id", func(g *PresentedGrant) { g.CapabilityGrantID = "grant-other" }},
		{"tenant", func(g *PresentedGrant) { g.TenantID = "tenant-other" }},
		{"task", func(g *PresentedGrant) { g.TaskID = "task-other" }},
		{"run", func(g *PresentedGrant) { g.RunID = "run-other" }},
		{"grantee-id", func(g *PresentedGrant) { g.Grantee.ID = "agent-other" }},
		{"grantee-type", func(g *PresentedGrant) { g.Grantee.Type = grantpolicy.ActorTypeService }},
		{"initiator-id", func(g *PresentedGrant) { g.Initiator.ID = "human-other" }},
		{"initiator-type", func(g *PresentedGrant) { g.Initiator.Type = grantpolicy.ActorTypeAgent }},
		{"capability", func(g *PresentedGrant) { g.Capabilities[0] = grantpolicy.CapabilityMessageReadHistory }},
		{"scope", func(g *PresentedGrant) { g.Scope.ChannelIDs[0] = "channel-other" }},
		{"issued-at", func(g *PresentedGrant) { g.IssuedAt = g.IssuedAt.Add(time.Nanosecond) }},
		{"expires-at", func(g *PresentedGrant) { g.ExpiresAt = g.ExpiresAt.Add(-time.Nanosecond) }},
		{"nonce", func(g *PresentedGrant) { g.Nonce[0] ^= 1 }},
		{"policy", func(g *PresentedGrant) { g.PolicyVersion = "policy-other" }},
		{"device", func(g *PresentedGrant) { g.ExecutionDeviceID = "device-other" }},
		{"profile", func(g *PresentedGrant) { g.SignatureProfile++ }},
		{"key-id", func(g *PresentedGrant) { g.SigningKeyID = "key-other" }},
		{"projection-version", func(g *PresentedGrant) { g.SignedProjectionVersion++ }},
		{"signature", func(g *PresentedGrant) { g.Signature[0] ^= 1 }},
	}

	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			mutated := cloneGrant(grant)
			testCase.mutate(&mutated)
			if _, err := verifier.VerifyIntegrity(grant.IssuedAt, mutated, expected); err == nil {
				t.Fatal("VerifyIntegrity() accepted a signed-field mutation")
			}
		})
	}
}

func TestVerifyIntegrityRejectsMalformedAndNoncanonicalGrant(t *testing.T) {
	t.Parallel()

	fixture := loadFixture(t)
	grant, verifier, expected := issueFixture(t, fixture)
	invalidUTF8 := string([]byte{0xff})
	cases := []struct {
		name   string
		mutate func(*PresentedGrant)
	}{
		{"invalid-utf8-identifier", func(g *PresentedGrant) { g.PolicyVersion = invalidUTF8 }},
		{"invalid-utf8-scope", func(g *PresentedGrant) { g.Scope.ToolIDs[0] = invalidUTF8 }},
		{"empty-device", func(g *PresentedGrant) { g.ExecutionDeviceID = "" }},
		{"unknown-actor", func(g *PresentedGrant) { g.Initiator.Type = 99 }},
		{"unknown-capability", func(g *PresentedGrant) { g.Capabilities[0] = 99 }},
		{"unsorted-capabilities", func(g *PresentedGrant) { g.Capabilities[0], g.Capabilities[1] = g.Capabilities[1], g.Capabilities[0] }},
		{"duplicate-capabilities", func(g *PresentedGrant) { g.Capabilities[1] = g.Capabilities[0] }},
		{"unsorted-scope", func(g *PresentedGrant) {
			g.Scope.WorkspacePathPrefixes[0], g.Scope.WorkspacePathPrefixes[1] = g.Scope.WorkspacePathPrefixes[1], g.Scope.WorkspacePathPrefixes[0]
		}},
		{"duplicate-scope", func(g *PresentedGrant) { g.Scope.WorkspacePathPrefixes[1] = g.Scope.WorkspacePathPrefixes[0] }},
		{"reversed-time", func(g *PresentedGrant) { g.ExpiresAt = g.IssuedAt }},
		{"short-nonce", func(g *PresentedGrant) { g.Nonce = g.Nonce[:31] }},
		{"short-signature", func(g *PresentedGrant) { g.Signature = g.Signature[:63] }},
		{"unknown-profile", func(g *PresentedGrant) { g.SignatureProfile = 99 }},
		{"unknown-version", func(g *PresentedGrant) { g.SignedProjectionVersion = 2 }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			mutated := cloneGrant(grant)
			testCase.mutate(&mutated)
			assertCredentialError(t, ErrorInvalidInput, func() error {
				_, err := verifier.VerifyIntegrity(grant.IssuedAt, mutated, expected)
				return err
			})
		})
	}
}

func TestVerifyIntegrityRequiresExactAuthenticatedAudienceAndTenantKey(t *testing.T) {
	t.Parallel()

	fixture := loadFixture(t)
	grant, verifier, expected := issueFixture(t, fixture)
	audienceCases := []struct {
		name   string
		mutate func(*ExpectedAudience)
	}{
		{"tenant", func(a *ExpectedAudience) { a.TenantID = "tenant-other" }},
		{"task", func(a *ExpectedAudience) { a.TaskID = "task-other" }},
		{"run", func(a *ExpectedAudience) { a.RunID = "run-other" }},
		{"grantee-id", func(a *ExpectedAudience) { a.Grantee.ID = "agent-other" }},
		{"grantee-type", func(a *ExpectedAudience) { a.Grantee.Type = grantpolicy.ActorTypeService }},
		{"device", func(a *ExpectedAudience) { a.ExecutionDeviceID = "device-other" }},
	}
	for _, testCase := range audienceCases {
		t.Run(testCase.name, func(t *testing.T) {
			mutated := expected
			testCase.mutate(&mutated)
			assertCredentialError(t, ErrorAudienceMismatch, func() error {
				_, err := verifier.VerifyIntegrity(grant.IssuedAt, grant, mutated)
				return err
			})
		})
	}

	otherKey := syntheticPrivateKey(0x70).Public().(ed25519.PublicKey)
	wrongVerifier, err := NewVerifier([]VerificationKey{
		{TenantID: expected.TenantID, KeyID: grant.SigningKeyID, PublicKey: otherKey},
		{TenantID: "tenant-other", KeyID: grant.SigningKeyID, PublicKey: syntheticPrivateKey(0x20).Public().(ed25519.PublicKey)},
	})
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	assertCredentialError(t, ErrorSignatureInvalid, func() error {
		_, verifyErr := wrongVerifier.VerifyIntegrity(grant.IssuedAt, grant, expected)
		return verifyErr
	})

	missingVerifier, err := NewVerifier([]VerificationKey{{
		TenantID: expected.TenantID, KeyID: "different-key", PublicKey: otherKey,
	}})
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	assertCredentialError(t, ErrorKeyUnavailable, func() error {
		_, verifyErr := missingVerifier.VerifyIntegrity(grant.IssuedAt, grant, expected)
		return verifyErr
	})
}

func TestVerifyIntegrityTimeBoundaries(t *testing.T) {
	t.Parallel()

	fixture := loadFixture(t)
	grant, verifier, expected := issueFixture(t, fixture)
	if _, err := verifier.VerifyIntegrity(grant.IssuedAt, grant, expected); err != nil {
		t.Fatalf("issued-at boundary rejected: %v", err)
	}
	assertCredentialError(t, ErrorNotYetValid, func() error {
		_, err := verifier.VerifyIntegrity(grant.IssuedAt.Add(-time.Nanosecond), grant, expected)
		return err
	})
	assertCredentialError(t, ErrorExpired, func() error {
		_, err := verifier.VerifyIntegrity(grant.ExpiresAt, grant, expected)
		return err
	})
	assertCredentialError(t, ErrorInvalidInput, func() error {
		_, err := verifier.VerifyIntegrity(time.Time{}, grant, expected)
		return err
	})
	outOfRange := cloneGrant(grant)
	outOfRange.IssuedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	assertCredentialError(t, ErrorInvalidInput, func() error {
		_, err := verifier.VerifyIntegrity(grant.IssuedAt, outOfRange, expected)
		return err
	})
}

func TestConstructorsAndIssuanceFailClosed(t *testing.T) {
	t.Parallel()

	fixture := loadFixture(t)
	privateKey := syntheticPrivateKey(0)
	badPrivateKey := append(ed25519.PrivateKey(nil), privateKey...)
	badPrivateKey[len(badPrivateKey)-1] ^= 1
	for _, testCase := range []struct {
		name string
		key  ed25519.PrivateKey
	}{
		{"short", privateKey[:len(privateKey)-1]},
		{"inconsistent", badPrivateKey},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NewIssuer("tenant-fixture-01", "key-fixture-01", testCase.key, bytes.NewReader(make([]byte, 32))); err == nil {
				t.Fatal("NewIssuer() accepted invalid private key")
			}
		})
	}
	if _, err := NewIssuer("tenant-fixture-01", "*", privateKey, bytes.NewReader(make([]byte, 32))); err == nil {
		t.Fatal("NewIssuer() accepted malformed key ID")
	}
	if _, err := NewVerifier([]VerificationKey{{TenantID: "tenant-a", KeyID: "key-a", PublicKey: privateKey.Public().(ed25519.PublicKey)}, {TenantID: "tenant-a", KeyID: "key-a", PublicKey: privateKey.Public().(ed25519.PublicKey)}}); err == nil {
		t.Fatal("NewVerifier() accepted duplicate tenant/key")
	}

	validated := validatedFixture(t, fixture.Valid.Grant)
	counting := &countingReader{err: io.ErrUnexpectedEOF}
	issuer, err := NewIssuer(fixture.Valid.Grant.TenantID, fixture.Valid.Grant.SigningKeyID, privateKey, counting)
	if err != nil {
		t.Fatalf("NewIssuer() error = %v", err)
	}
	assertEntropyFailure := func(t *testing.T, failingIssuer *Issuer) {
		t.Helper()
		grant, issueErr := failingIssuer.Issue(mustTime(t, fixture.Valid.Grant.IssuedAt), fixture.Valid.Grant.CapabilityGrantID, validated)
		assertCredentialError(t, ErrorEntropyUnavailable, func() error { return issueErr })
		if !reflect.DeepEqual(grant, PresentedGrant{}) {
			t.Fatal("entropy failure returned a partial Grant")
		}
	}
	assertEntropyFailure(t, issuer)

	for _, entropy := range []io.Reader{bytes.NewReader(make([]byte, nonceSize-1)), panicReader{}} {
		failingIssuer, issueErr := NewIssuer(fixture.Valid.Grant.TenantID, fixture.Valid.Grant.SigningKeyID, privateKey, entropy)
		if issueErr != nil {
			t.Fatalf("NewIssuer() error = %v", issueErr)
		}
		assertEntropyFailure(t, failingIssuer)
	}

	otherTenant := validatedFixtureForTenant(t, fixture.Valid.Grant, "tenant-other")
	counting.err = nil
	counting.reads = 0
	assertCredentialError(t, ErrorInvalidInput, func() error {
		_, issueErr := issuer.Issue(mustTime(t, fixture.Valid.Grant.IssuedAt), fixture.Valid.Grant.CapabilityGrantID, otherTenant)
		return issueErr
	})
	if counting.reads != 0 {
		t.Fatal("invalid request consumed entropy")
	}
}

func TestErrorCategoriesNeverIncludeInputs(t *testing.T) {
	t.Parallel()

	const secret = "secret-tenant/private-key/nonce/signature"
	fixture := loadFixture(t)
	privateKey := syntheticPrivateKey(0)
	validated := validatedFixture(t, fixture.Valid.Grant)
	at := mustTime(t, fixture.Valid.Grant.IssuedAt)
	grant, verifier, expected := issueFixture(t, fixture)

	_, configurationErr := NewIssuer(fixture.Valid.Grant.TenantID, secret+"*", privateKey, bytes.NewReader(fixtureEntropy()))
	goodIssuer, err := NewIssuer(fixture.Valid.Grant.TenantID, fixture.Valid.Grant.SigningKeyID, privateKey, bytes.NewReader(fixtureEntropy()))
	if err != nil {
		t.Fatalf("NewIssuer() error = %v", err)
	}
	_, inputErr := goodIssuer.Issue(at, secret+"*", validated)
	entropyIssuer, err := NewIssuer(fixture.Valid.Grant.TenantID, fixture.Valid.Grant.SigningKeyID, privateKey, &countingReader{err: errors.New(secret)})
	if err != nil {
		t.Fatalf("NewIssuer() error = %v", err)
	}
	_, entropyErr := entropyIssuer.Issue(at, fixture.Valid.Grant.CapabilityGrantID, validated)
	wrongAudience := expected
	wrongAudience.ExecutionDeviceID = secret
	_, audienceErr := verifier.VerifyIntegrity(at, grant, wrongAudience)
	missingKey := cloneGrant(grant)
	missingKey.SigningKeyID = secret
	_, keyErr := verifier.VerifyIntegrity(at, missingKey, expected)
	_, futureErr := verifier.VerifyIntegrity(at.Add(-time.Nanosecond), grant, expected)
	_, expiredErr := verifier.VerifyIntegrity(grant.ExpiresAt, grant, expected)
	badSignature := cloneGrant(grant)
	badSignature.Signature[0] ^= 1
	_, signatureErr := verifier.VerifyIntegrity(at, badSignature, expected)

	for _, testCase := range []struct {
		code ErrorCode
		err  error
	}{
		{ErrorInvalidConfiguration, configurationErr},
		{ErrorInvalidInput, inputErr},
		{ErrorEntropyUnavailable, entropyErr},
		{ErrorAudienceMismatch, audienceErr},
		{ErrorKeyUnavailable, keyErr},
		{ErrorNotYetValid, futureErr},
		{ErrorExpired, expiredErr},
		{ErrorSignatureInvalid, signatureErr},
	} {
		assertCredentialError(t, testCase.code, func() error { return testCase.err })
		want := "capability grant credential: " + string(testCase.code)
		if got := testCase.err.Error(); got != want || strings.Contains(got, secret) {
			t.Fatalf("unstable or input-bearing error for code %q", testCase.code)
		}
	}
}

func TestIssuerVerifierAndVerifiedFactsDefensivelyCopyInputs(t *testing.T) {
	t.Parallel()

	fixture := loadFixture(t)
	privateKey := syntheticPrivateKey(0)
	publicKey := append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...)
	issuer, err := NewIssuer(fixture.Valid.Grant.TenantID, fixture.Valid.Grant.SigningKeyID, privateKey, bytes.NewReader(fixtureEntropy()))
	if err != nil {
		t.Fatalf("NewIssuer() error = %v", err)
	}
	verifier, err := NewVerifier([]VerificationKey{{TenantID: fixture.Valid.Grant.TenantID, KeyID: fixture.Valid.Grant.SigningKeyID, PublicKey: publicKey}})
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	privateKey[0] ^= 1
	publicKey[0] ^= 1

	grant, issueErr := issuer.Issue(mustTime(t, fixture.Valid.Grant.IssuedAt), fixture.Valid.Grant.CapabilityGrantID, validatedFixture(t, fixture.Valid.Grant))
	if issueErr != nil {
		t.Fatalf("Issue() error = %v", issueErr)
	}
	expected := expectedFixture(fixture.Valid.ExpectedAudience)
	verified, verifyErr := verifier.VerifyIntegrity(grant.IssuedAt, grant, expected)
	if verifyErr != nil {
		t.Fatalf("VerifyIntegrity() error = %v", verifyErr)
	}
	wantCapabilities := verified.Capabilities()
	wantScope := verified.Scope()
	grant.Capabilities[0] = grantpolicy.CapabilityAuditRead
	grant.Scope.ChannelIDs[0] = "mutated"
	grant.Nonce[0] ^= 1
	grant.Signature[0] ^= 1
	gotCapabilities := verified.Capabilities()
	gotScope := verified.Scope()
	if !reflect.DeepEqual(gotCapabilities, wantCapabilities) || !reflect.DeepEqual(gotScope, wantScope) {
		t.Fatal("presented Grant mutation changed verified facts")
	}
	gotCapabilities[0] = grantpolicy.CapabilityAuditRead
	gotScope.ChannelIDs[0] = "mutated-again"
	if reflect.DeepEqual(verified.Capabilities(), gotCapabilities) || reflect.DeepEqual(verified.Scope(), gotScope) {
		t.Fatal("verified accessors expose mutable internal slices")
	}
}

func TestIssuerSerializesConcurrentEntropyReads(t *testing.T) {
	fixture := loadFixture(t)
	privateKey := syntheticPrivateKey(0)
	entropy := &unsafeSequenceReader{}
	issuer, err := NewIssuer(fixture.Valid.Grant.TenantID, fixture.Valid.Grant.SigningKeyID, privateKey, entropy)
	if err != nil {
		t.Fatalf("NewIssuer() error = %v", err)
	}
	validated := validatedFixture(t, fixture.Valid.Grant)
	at := mustTime(t, fixture.Valid.Grant.IssuedAt)

	const workers = 16
	var wait sync.WaitGroup
	errorsByWorker := make(chan error, workers)
	for worker := range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, issueErr := issuer.Issue(at, "grant-concurrent-"+string(rune('a'+worker)), validated)
			errorsByWorker <- issueErr
		}()
	}
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatalf("concurrent Issue() error = %v", err)
		}
	}
}

type countingReader struct {
	reads int
	err   error
}

func (r *countingReader) Read(buffer []byte) (int, error) {
	r.reads++
	if r.err != nil {
		return 0, r.err
	}
	for index := range buffer {
		buffer[index] = byte(index)
	}
	return len(buffer), nil
}

type unsafeSequenceReader struct{ next byte }

func (r *unsafeSequenceReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = r.next
		r.next++
	}
	return len(buffer), nil
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("secret-tenant/private-key/nonce/signature") }

func issueFixture(t *testing.T, fixture fixtureFile) (PresentedGrant, *Verifier, ExpectedAudience) {
	t.Helper()
	privateKey := syntheticPrivateKey(0)
	issuer, err := NewIssuer(fixture.Valid.Grant.TenantID, fixture.Valid.Grant.SigningKeyID, privateKey, bytes.NewReader(fixtureEntropy()))
	if err != nil {
		t.Fatalf("NewIssuer() error = %v", err)
	}
	issued, err := issuer.Issue(mustTime(t, fixture.Valid.Grant.IssuedAt), fixture.Valid.Grant.CapabilityGrantID, validatedFixture(t, fixture.Valid.Grant))
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	publicKey, err := hex.DecodeString(fixture.Valid.PublicKeyRawHex)
	if err != nil {
		t.Fatalf("decode fixture public key: %v", err)
	}
	verifier, err := NewVerifier([]VerificationKey{{TenantID: issued.TenantID, KeyID: issued.SigningKeyID, PublicKey: publicKey}})
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	return issued, verifier, expectedFixture(fixture.Valid.ExpectedAudience)
}

func validatedFixture(t *testing.T, fixture fixtureGrant) grantpolicy.ValidatedRequest {
	return validatedFixtureForTenant(t, fixture, fixture.TenantID)
}

func validatedFixtureForTenant(t *testing.T, fixture fixtureGrant, tenantID string) grantpolicy.ValidatedRequest {
	t.Helper()
	issuedAt := mustTime(t, fixture.IssuedAt)
	expiresAt := mustTime(t, fixture.ExpiresAt)
	capabilities := make([]grantpolicy.Capability, len(fixture.Capabilities))
	for index, capability := range fixture.Capabilities {
		capabilities[index] = grantpolicy.Capability(capability)
	}
	scope := scopeFixture(fixture.ResourceScope)
	validated, err := grantpolicy.ValidateAndNarrow(issuedAt, grantpolicy.Request{
		Capabilities: capabilities,
		Scope:        scope,
		ExpiresAt:    expiresAt,
	}, grantpolicy.IssuanceAuthority{
		TenantID:      tenantID,
		TaskID:        fixture.TaskID,
		RunID:         fixture.RunID,
		DeviceID:      fixture.ExecutionDeviceID,
		Grantee:       actorFixture(fixture.Grantee),
		Initiator:     actorFixture(fixture.Initiator),
		PolicyVersion: fixture.PolicyVersion,
		Capabilities:  capabilities,
		Scope:         scope,
		NotAfter:      expiresAt,
	})
	if err != nil {
		t.Fatalf("ValidateAndNarrow() error = %v", err)
	}
	return validated
}

func presentedFixture(t *testing.T, fixture fixtureGrant) PresentedGrant {
	t.Helper()
	nonce, err := hex.DecodeString(fixture.NonceHex)
	if err != nil {
		t.Fatalf("decode fixture nonce: %v", err)
	}
	signature, err := hex.DecodeString(fixture.SignatureHex)
	if err != nil {
		t.Fatalf("decode fixture signature: %v", err)
	}
	capabilities := make([]grantpolicy.Capability, len(fixture.Capabilities))
	for index, capability := range fixture.Capabilities {
		capabilities[index] = grantpolicy.Capability(capability)
	}
	return PresentedGrant{
		CapabilityGrantID:       fixture.CapabilityGrantID,
		TenantID:                fixture.TenantID,
		TaskID:                  fixture.TaskID,
		RunID:                   fixture.RunID,
		Grantee:                 actorFixture(fixture.Grantee),
		Initiator:               actorFixture(fixture.Initiator),
		Capabilities:            capabilities,
		Scope:                   scopeFixture(fixture.ResourceScope),
		IssuedAt:                mustTime(t, fixture.IssuedAt),
		ExpiresAt:               mustTime(t, fixture.ExpiresAt),
		Nonce:                   nonce,
		PolicyVersion:           fixture.PolicyVersion,
		Signature:               signature,
		ExecutionDeviceID:       fixture.ExecutionDeviceID,
		SignatureProfile:        signatureProfileEd25519JCSV1,
		SigningKeyID:            fixture.SigningKeyID,
		SignedProjectionVersion: fixture.SignedProjectionVersion,
	}
}

func actorFixture(actor fixtureActor) grantpolicy.ActorRef {
	return grantpolicy.ActorRef{Type: grantpolicy.ActorType(actor.ActorType), ID: actor.ActorID}
}

func scopeFixture(scope fixtureScope) grantpolicy.ResourceScope {
	return grantpolicy.ResourceScope{
		ChannelIDs:            append([]string(nil), scope.ChannelIDs...),
		DMIDs:                 append([]string(nil), scope.DMIDs...),
		EventIDs:              append([]string(nil), scope.EventIDs...),
		ThreadIDs:             append([]string(nil), scope.ThreadIDs...),
		FileIDs:               append([]string(nil), scope.FileIDs...),
		WorkspaceBindingIDs:   append([]string(nil), scope.WorkspaceBindingIDs...),
		WorkspacePathPrefixes: append([]string(nil), scope.WorkspacePathPrefixes...),
		ToolIDs:               append([]string(nil), scope.ToolIDs...),
	}
}

func expectedFixture(expected fixtureAudience) ExpectedAudience {
	return ExpectedAudience{
		TenantID: expected.TenantID, TaskID: expected.TaskID, RunID: expected.RunID,
		Grantee: actorFixture(expected.Grantee), ExecutionDeviceID: expected.ExecutionDeviceID,
	}
}

func syntheticPrivateKey(start byte) ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = start + byte(index)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func fixtureEntropy() []byte {
	entropy := make([]byte, nonceSize)
	for index := range entropy {
		entropy[index] = 0x20 + byte(index)
	}
	return entropy
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02T15:04:05.000000000Z", value)
	if err != nil {
		t.Fatalf("parse fixture time: %v", err)
	}
	return parsed
}

func loadFixture(t *testing.T) fixtureFile {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "test", "fixtures", "proto", "capability-grant", "vectors.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read public fixture: %v", err)
	}
	var fixture fixtureFile
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode public fixture: %v", err)
	}
	return fixture
}

func assertCredentialError(t *testing.T, want ErrorCode, action func() error) {
	t.Helper()
	err := action()
	var credentialError *Error
	if !errors.As(err, &credentialError) || credentialError.Code() != want {
		t.Fatalf("error = %v, want code %q", err, want)
	}
}

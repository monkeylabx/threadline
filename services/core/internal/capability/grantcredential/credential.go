package grantcredential

import (
	"bytes"
	"crypto/ed25519"
	"io"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/monkeylabx/threadline/services/core/internal/capability/grantpolicy"
)

const nonceSize = 32

// NewIssuer constructs one tenant-bound in-process issuer. It copies the key
// and takes exclusive ownership of entropy; callers must not use entropy again.
func NewIssuer(
	tenantID string,
	keyID string,
	privateKey ed25519.PrivateKey,
	entropy io.Reader,
) (*Issuer, error) {
	if !validIdentifier(tenantID) || !validIdentifier(keyID) || entropy == nil ||
		len(privateKey) != ed25519.PrivateKeySize {
		return nil, &Error{code: ErrorInvalidConfiguration}
	}
	canonicalKey := ed25519.NewKeyFromSeed(privateKey.Seed())
	if !bytes.Equal(canonicalKey, privateKey) {
		return nil, &Error{code: ErrorInvalidConfiguration}
	}
	return &Issuer{
		tenantID:   tenantID,
		keyID:      keyID,
		privateKey: append(ed25519.PrivateKey(nil), privateKey...),
		entropy:    entropy,
	}, nil
}

// Issue creates one signed Grant from a P03-06A validated request. It never
// returns a partial credential.
func (i *Issuer) Issue(
	at time.Time,
	grantID string,
	request grantpolicy.ValidatedRequest,
) (PresentedGrant, error) {
	if i == nil || !validInstant(at) || !validIdentifier(grantID) ||
		request.TenantID() != i.tenantID {
		return PresentedGrant{}, &Error{code: ErrorInvalidInput}
	}

	nonce := make([]byte, nonceSize)
	grant := PresentedGrant{
		CapabilityGrantID:       grantID,
		TenantID:                request.TenantID(),
		TaskID:                  request.TaskID(),
		RunID:                   request.RunID(),
		Grantee:                 request.Grantee(),
		Initiator:               request.Initiator(),
		Capabilities:            request.Capabilities(),
		Scope:                   request.Scope(),
		IssuedAt:                at.UTC(),
		ExpiresAt:               request.ExpiresAt().UTC(),
		Nonce:                   nonce,
		PolicyVersion:           request.PolicyVersion(),
		ExecutionDeviceID:       request.DeviceID(),
		SignatureProfile:        signatureProfileEd25519JCSV1,
		SigningKeyID:            i.keyID,
		SignedProjectionVersion: signedProjectionVersionV1,
	}
	if err := validateGrantClaims(grant); err != nil {
		return PresentedGrant{}, err
	}
	if err := i.readNonce(nonce); err != nil {
		return PresentedGrant{}, err
	}
	transcript, err := canonicalTranscript(grant)
	if err != nil {
		return PresentedGrant{}, err
	}
	grant.Signature = ed25519.Sign(i.privateKey, transcript)
	return cloneGrant(grant), nil
}

func (i *Issuer) readNonce(nonce []byte) (err error) {
	i.entropyMu.Lock()
	defer i.entropyMu.Unlock()
	defer func() {
		if recover() != nil {
			err = &Error{code: ErrorEntropyUnavailable}
		}
	}()
	if _, readErr := io.ReadFull(i.entropy, nonce); readErr != nil {
		return &Error{code: ErrorEntropyUnavailable}
	}
	return nil
}

// NewVerifier constructs an immutable tenant-scoped verification key ring.
func NewVerifier(keys []VerificationKey) (*Verifier, error) {
	if len(keys) == 0 {
		return nil, &Error{code: ErrorInvalidConfiguration}
	}
	keyRing := make(map[string]map[string]ed25519.PublicKey)
	for _, key := range keys {
		if !validIdentifier(key.TenantID) || !validIdentifier(key.KeyID) ||
			len(key.PublicKey) != ed25519.PublicKeySize {
			return nil, &Error{code: ErrorInvalidConfiguration}
		}
		tenantKeys := keyRing[key.TenantID]
		if tenantKeys == nil {
			tenantKeys = make(map[string]ed25519.PublicKey)
			keyRing[key.TenantID] = tenantKeys
		}
		if _, duplicate := tenantKeys[key.KeyID]; duplicate {
			return nil, &Error{code: ErrorInvalidConfiguration}
		}
		tenantKeys[key.KeyID] = append(ed25519.PublicKey(nil), key.PublicKey...)
	}
	return &Verifier{keys: keyRing}, nil
}

// VerifyIntegrity checks structural canonicality, authenticated audience,
// local time bounds, tenant-scoped key selection, and the Ed25519 signature.
// The returned value still requires current Core lifecycle/authorization check.
func (v *Verifier) VerifyIntegrity(
	at time.Time,
	grant PresentedGrant,
	expected ExpectedAudience,
) (VerifiedGrant, error) {
	if v == nil || !validInstant(at) {
		return VerifiedGrant{}, &Error{code: ErrorInvalidInput}
	}
	if err := validateGrantStructure(grant); err != nil {
		return VerifiedGrant{}, err
	}
	if !validAudience(expected) {
		return VerifiedGrant{}, &Error{code: ErrorInvalidInput}
	}
	if grant.TenantID != expected.TenantID || grant.TaskID != expected.TaskID ||
		grant.RunID != expected.RunID || grant.Grantee != expected.Grantee ||
		grant.ExecutionDeviceID != expected.ExecutionDeviceID {
		return VerifiedGrant{}, &Error{code: ErrorAudienceMismatch}
	}
	tenantKeys := v.keys[expected.TenantID]
	publicKey, ok := tenantKeys[grant.SigningKeyID]
	if !ok {
		return VerifiedGrant{}, &Error{code: ErrorKeyUnavailable}
	}
	if at.Before(grant.IssuedAt) {
		return VerifiedGrant{}, &Error{code: ErrorNotYetValid}
	}
	if !at.Before(grant.ExpiresAt) {
		return VerifiedGrant{}, &Error{code: ErrorExpired}
	}
	transcript, err := canonicalTranscript(grant)
	if err != nil {
		return VerifiedGrant{}, err
	}
	if !ed25519.Verify(publicKey, transcript, grant.Signature) {
		return VerifiedGrant{}, &Error{code: ErrorSignatureInvalid}
	}
	return VerifiedGrant{grant: cloneGrant(grant)}, nil
}

func validateGrantStructure(grant PresentedGrant) error {
	if err := validateGrantClaims(grant); err != nil {
		return err
	}
	if len(grant.Signature) != ed25519.SignatureSize {
		return &Error{code: ErrorInvalidInput}
	}
	return nil
}

func validateGrantClaims(grant PresentedGrant) error {
	for _, value := range []string{
		grant.CapabilityGrantID,
		grant.TenantID,
		grant.TaskID,
		grant.RunID,
		grant.PolicyVersion,
		grant.ExecutionDeviceID,
		grant.SigningKeyID,
	} {
		if !validIdentifier(value) {
			return &Error{code: ErrorInvalidInput}
		}
	}
	if !validActor(grant.Grantee) || !validActor(grant.Initiator) ||
		!validCapabilities(grant.Capabilities) || !validScope(grant.Scope) ||
		!validInstant(grant.IssuedAt) || !validInstant(grant.ExpiresAt) ||
		!grant.IssuedAt.Before(grant.ExpiresAt) || len(grant.Nonce) != nonceSize ||
		grant.SignatureProfile != signatureProfileEd25519JCSV1 ||
		grant.SignedProjectionVersion != signedProjectionVersionV1 {
		return &Error{code: ErrorInvalidInput}
	}
	return nil
}

func validAudience(expected ExpectedAudience) bool {
	return validIdentifier(expected.TenantID) && validIdentifier(expected.TaskID) &&
		validIdentifier(expected.RunID) && validActor(expected.Grantee) &&
		validIdentifier(expected.ExecutionDeviceID)
}

func validActor(actor grantpolicy.ActorRef) bool {
	return actor.Type >= grantpolicy.ActorTypeHuman && actor.Type <= grantpolicy.ActorTypeService &&
		validIdentifier(actor.ID)
}

func validCapabilities(capabilities []grantpolicy.Capability) bool {
	if len(capabilities) == 0 || !slices.IsSorted(capabilities) {
		return false
	}
	for index, capability := range capabilities {
		if capability < grantpolicy.CapabilityMessageRead || capability > grantpolicy.CapabilityAuditRead ||
			(index > 0 && capabilities[index-1] == capability) {
			return false
		}
	}
	return true
}

func validScope(scope grantpolicy.ResourceScope) bool {
	return validSortedIdentifiers(scope.ChannelIDs) && validSortedIdentifiers(scope.DMIDs) &&
		validSortedIdentifiers(scope.EventIDs) && validSortedIdentifiers(scope.ThreadIDs) &&
		validSortedIdentifiers(scope.FileIDs) && validSortedIdentifiers(scope.WorkspaceBindingIDs) &&
		validSortedIdentifiers(scope.WorkspacePathPrefixes) && validSortedIdentifiers(scope.ToolIDs)
}

func validSortedIdentifiers(values []string) bool {
	if !slices.IsSorted(values) {
		return false
	}
	for index, value := range values {
		if !validIdentifier(value) || (index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}

func validIdentifier(value string) bool {
	if !utf8.ValidString(value) || value == "" || strings.TrimSpace(value) != value ||
		strings.ContainsAny(value, "*?") {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validInstant(value time.Time) bool {
	return !value.IsZero() && value.Year() >= 1 && value.Year() <= 9999
}

func cloneGrant(grant PresentedGrant) PresentedGrant {
	grant.Capabilities = append([]grantpolicy.Capability(nil), grant.Capabilities...)
	grant.Scope = cloneScope(grant.Scope)
	grant.Nonce = append([]byte(nil), grant.Nonce...)
	grant.Signature = append([]byte(nil), grant.Signature...)
	return grant
}

func cloneScope(scope grantpolicy.ResourceScope) grantpolicy.ResourceScope {
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

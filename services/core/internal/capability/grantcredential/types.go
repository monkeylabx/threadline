// Package grantcredential issues and verifies immutable Capability Grant
// credentials. It proves signed claim integrity and authenticated audience
// binding only; current lifecycle and authorization facts require a separate
// Core recheck before every protected use.
package grantcredential

import (
	"crypto/ed25519"
	"io"
	"sync"
	"time"

	"github.com/monkeylabx/threadline/services/core/internal/capability/grantpolicy"
)

const (
	signatureProfileEd25519JCSV1 int32  = 1
	signedProjectionVersionV1    uint32 = 1
)

// PresentedGrant is an untrusted decoded credential. Callers may construct or
// mutate it; Verifier validates and copies every field before returning facts.
type PresentedGrant struct {
	CapabilityGrantID       string
	TenantID                string
	TaskID                  string
	RunID                   string
	Grantee                 grantpolicy.ActorRef
	Initiator               grantpolicy.ActorRef
	Capabilities            []grantpolicy.Capability
	Scope                   grantpolicy.ResourceScope
	IssuedAt                time.Time
	ExpiresAt               time.Time
	Nonce                   []byte
	PolicyVersion           string
	Signature               []byte
	ExecutionDeviceID       string
	SignatureProfile        int32
	SigningKeyID            string
	SignedProjectionVersion uint32
}

// ExpectedAudience contains independently authenticated facts for the current
// use. None of these values may be copied from PresentedGrant.
type ExpectedAudience struct {
	TenantID          string
	TaskID            string
	RunID             string
	Grantee           grantpolicy.ActorRef
	ExecutionDeviceID string
}

// VerificationKey is one trusted tenant-scoped Ed25519 public key.
type VerificationKey struct {
	TenantID  string
	KeyID     string
	PublicKey ed25519.PublicKey
}

// Issuer is a tenant-bound in-process signing module. Key material is copied.
// Ownership of the entropy reader is transferred exclusively to the Issuer;
// callers must not retain or access it after construction.
type Issuer struct {
	tenantID   string
	keyID      string
	privateKey ed25519.PrivateKey
	entropy    io.Reader
	entropyMu  sync.Mutex
}

// Verifier owns an immutable tenant/key lookup copied at construction.
type Verifier struct {
	keys map[string]map[string]ed25519.PublicKey
}

// VerifiedGrant contains immutable facts whose signature, local time bounds,
// and authenticated audience were verified. It does not assert current state.
type VerifiedGrant struct {
	grant PresentedGrant
}

func (v VerifiedGrant) CapabilityGrantID() string       { return v.grant.CapabilityGrantID }
func (v VerifiedGrant) TenantID() string                { return v.grant.TenantID }
func (v VerifiedGrant) TaskID() string                  { return v.grant.TaskID }
func (v VerifiedGrant) RunID() string                   { return v.grant.RunID }
func (v VerifiedGrant) Grantee() grantpolicy.ActorRef   { return v.grant.Grantee }
func (v VerifiedGrant) Initiator() grantpolicy.ActorRef { return v.grant.Initiator }
func (v VerifiedGrant) ExecutionDeviceID() string       { return v.grant.ExecutionDeviceID }
func (v VerifiedGrant) PolicyVersion() string           { return v.grant.PolicyVersion }
func (v VerifiedGrant) IssuedAt() time.Time             { return v.grant.IssuedAt }
func (v VerifiedGrant) ExpiresAt() time.Time            { return v.grant.ExpiresAt }
func (v VerifiedGrant) Capabilities() []grantpolicy.Capability {
	return append([]grantpolicy.Capability(nil), v.grant.Capabilities...)
}
func (v VerifiedGrant) Scope() grantpolicy.ResourceScope { return cloneScope(v.grant.Scope) }

// ErrorCode is a stable, secret-safe failure category.
type ErrorCode string

const (
	ErrorInvalidConfiguration ErrorCode = "invalid-configuration"
	ErrorInvalidInput         ErrorCode = "invalid-input"
	ErrorEntropyUnavailable   ErrorCode = "entropy-unavailable"
	ErrorAudienceMismatch     ErrorCode = "audience-mismatch"
	ErrorKeyUnavailable       ErrorCode = "key-unavailable"
	ErrorNotYetValid          ErrorCode = "not-yet-valid"
	ErrorExpired              ErrorCode = "expired"
	ErrorSignatureInvalid     ErrorCode = "signature-invalid"
)

// Error contains no wrapped diagnostic or input value.
type Error struct{ code ErrorCode }

func (e *Error) Error() string { return "capability grant credential: " + string(e.Code()) }

func (e *Error) Code() ErrorCode {
	if e == nil {
		return ""
	}
	return e.code
}

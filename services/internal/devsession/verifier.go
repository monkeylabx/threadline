package devsession

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

// Options configure the dev verifier. Enabled must be set explicitly by the
// operator; a zero Options never constructs a verifier.
type Options struct {
	Enabled  bool
	Fixtures []Fixture
	Now      func() time.Time
}

// Verifier answers VerifySession from static fixtures. NOT PRODUCTION.
type Verifier struct {
	fixtures []Fixture
	now      func() time.Time
}

// New refuses to construct unless the dev flag is on and at least one fixture
// is present, so the verifier cannot appear by default.
func New(options Options) (*Verifier, error) {
	if !options.Enabled {
		return nil, errors.New("devsession: dev sessions are not enabled")
	}
	if len(options.Fixtures) == 0 {
		return nil, errors.New("devsession: no fixtures")
	}
	for _, fixture := range options.Fixtures {
		if !validFixture(fixture) {
			return nil, errors.New("devsession: fixture invalid")
		}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Verifier{fixtures: append([]Fixture(nil), options.Fixtures...), now: now}, nil
}

// VerifySession compares the credential against every fixture in constant
// time per fixture, then reports revocation or expiry before success. Unknown
// credentials are rejected; the credential never appears in any error.
func (verifier *Verifier) VerifySession(ctx context.Context, credential string) (rpcmiddleware.VerifiedSession, error) {
	if err := ctx.Err(); err != nil {
		return rpcmiddleware.VerifiedSession{}, err
	}
	match, found := verifier.lookup(credential)
	if !found {
		return rpcmiddleware.VerifiedSession{}, rpcmiddleware.NewVerificationError(rpcmiddleware.VerificationRejected)
	}
	if match.Revoked {
		return rpcmiddleware.VerifiedSession{}, rpcmiddleware.NewVerificationError(rpcmiddleware.VerificationRevoked)
	}
	if !match.ExpiresAt.IsZero() && !verifier.now().Before(match.ExpiresAt) {
		return rpcmiddleware.VerifiedSession{}, rpcmiddleware.NewVerificationError(rpcmiddleware.VerificationExpired)
	}
	return rpcmiddleware.VerifiedSession{
		TenantID:  match.TenantID,
		ActorType: actorTypes[match.ActorType],
		ActorID:   match.ActorID,
		DeviceID:  match.DeviceID,
		SessionID: match.SessionID,
	}, nil
}

func (verifier *Verifier) lookup(credential string) (Fixture, bool) {
	var match Fixture
	found := false
	for _, fixture := range verifier.fixtures {
		if subtle.ConstantTimeCompare([]byte(fixture.Token), []byte(credential)) == 1 {
			match, found = fixture, true
		}
	}
	return match, found
}

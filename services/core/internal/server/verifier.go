package server

import (
	"context"

	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

// RejectAllVerifier rejects every bearer credential. It is the process default
// so a core binary started without a configured verifier fails closed instead
// of trusting anyone. The Slice A dev verifier (#267) and the production
// session service (#192/#200) replace it through server.Dependencies.
type RejectAllVerifier struct{}

// VerifySession reports VerificationRejected without inspecting the credential.
func (RejectAllVerifier) VerifySession(context.Context, string) (rpcmiddleware.VerifiedSession, error) {
	return rpcmiddleware.VerifiedSession{},
		rpcmiddleware.NewVerificationError(rpcmiddleware.VerificationRejected)
}

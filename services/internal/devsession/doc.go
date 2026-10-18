// Package devsession is a NOT PRODUCTION session verifier for the Slice A
// development stack. It answers rpcmiddleware.SessionVerifier from a small,
// explicitly enabled fixture document of static bearer tokens so two Desktop
// clients can authenticate against a local core process before the real
// Session service exists.
//
// It must never ship in a production configuration: it is constructed only
// when the operator sets both the enable flag and a fixture document, its
// fixtures are static secrets for a disposable dev stack, and the production
// path remains docs/contracts/session-device-verification.md (#192) and
// docs/contracts/device-proof-bootstrap.md (#200).
package devsession

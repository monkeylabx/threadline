// Package rpcmiddleware authenticates every server-side Connect procedure and
// exposes the resulting immutable Principal through request context.
//
// SessionVerifier is the module's adapter seam. Tests use an in-memory adapter;
// P03-02 will supply the production session verifier without changing handlers.
// Request-body or request-header tenant IDs are resource facts only: they must
// never be copied into Principal or treated as authenticated identity. P03-05
// must compare those facts with Principal.TenantID before authorizing access.
package rpcmiddleware

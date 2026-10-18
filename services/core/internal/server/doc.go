// Package server assembles the threadline-core process: validated
// configuration, a health endpoint backed by the PostgreSQL pool, and every
// Connect service registered behind the authentication interceptor.
//
// The package owns composition only. Authorization lives in the service
// handlers and session verification behind rpcmiddleware.SessionVerifier.
package server

package rpcmiddleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"connectrpc.com/connect"
)

// ActorType identifies the authenticated kind of actor.
type ActorType uint8

const (
	ActorTypeHuman ActorType = iota + 1
	ActorTypeAgent
	ActorTypeService
)

// VerifiedSession is the validated identity material returned by a
// SessionVerifier. The interceptor validates it again before constructing a
// Principal so a faulty adapter fails closed.
type VerifiedSession struct {
	TenantID  string
	ActorType ActorType
	ActorID   string
	DeviceID  string
	SessionID string
}

// SessionVerifier verifies a raw bearer credential. Implementations must not
// retain, log, or include bearerCredential in returned errors.
type SessionVerifier interface {
	VerifySession(context.Context, string) (VerifiedSession, error)
}

// VerificationFailure is an authentication rejection understood by the
// interceptor. Other verifier errors are treated as infrastructure failures.
type VerificationFailure uint8

const (
	VerificationRejected VerificationFailure = iota + 1
	VerificationExpired
	VerificationRevoked
)

// VerificationError reports a typed authentication rejection without carrying
// adapter error text or credential material.
type VerificationError struct {
	failure VerificationFailure
}

// NewVerificationError creates a secret-safe typed verifier failure.
func NewVerificationError(failure VerificationFailure) *VerificationError {
	return &VerificationError{failure: failure}
}

// Error implements error with a stable, non-sensitive message.
func (*VerificationError) Error() string {
	return "session verification failed"
}

// Failure returns the rejection category.
func (e *VerificationError) Failure() VerificationFailure {
	if e == nil {
		return 0
	}
	return e.failure
}

// ActorRef is the immutable actor component of a Principal.
type ActorRef struct {
	actorType ActorType
	id        string
}

// Type returns the authenticated actor type.
func (a ActorRef) Type() ActorType { return a.actorType }

// ID returns the authenticated actor identifier.
func (a ActorRef) ID() string { return a.id }

// Principal is the immutable identity authenticated for one RPC. It contains
// no bearer credential and cannot be constructed by ordinary callers.
type Principal struct {
	tenantID  string
	actor     ActorRef
	deviceID  string
	sessionID string
}

// TenantID returns the authenticated tenant identifier.
func (p Principal) TenantID() string { return p.tenantID }

// Actor returns the authenticated actor reference.
func (p Principal) Actor() ActorRef { return p.actor }

// DeviceID returns the authenticated device identifier.
func (p Principal) DeviceID() string { return p.deviceID }

// SessionID returns the authenticated session identifier.
func (p Principal) SessionID() string { return p.sessionID }

type principalContextKey struct{}

// PrincipalFromContext returns the Principal injected by the authentication
// interceptor. A false result means the request has not crossed that boundary.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

type authenticationInterceptor struct {
	verifier SessionVerifier
}

// NewAuthenticationInterceptor constructs a Connect interceptor that
// authenticates every unary and streaming handler procedure. Its streaming
// client method is an explicit pass-through because client auth is out of scope.
func NewAuthenticationInterceptor(verifier SessionVerifier) connect.Interceptor {
	return &authenticationInterceptor{verifier: verifier}
}

func (i *authenticationInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		principal, err := i.authenticate(ctx, request.Header())
		if err != nil {
			return nil, err
		}
		return next(context.WithValue(ctx, principalContextKey{}, principal), request)
	}
}

func (i *authenticationInterceptor) WrapStreamingClient(
	next connect.StreamingClientFunc,
) connect.StreamingClientFunc {
	return next
}

func (i *authenticationInterceptor) WrapStreamingHandler(
	next connect.StreamingHandlerFunc,
) connect.StreamingHandlerFunc {
	return func(ctx context.Context, connection connect.StreamingHandlerConn) error {
		principal, err := i.authenticate(ctx, connection.RequestHeader())
		if err != nil {
			return err
		}
		return next(context.WithValue(ctx, principalContextKey{}, principal), connection)
	}
}

func (i *authenticationInterceptor) authenticate(
	ctx context.Context,
	header http.Header,
) (Principal, error) {
	credential, headerErr := readBearerCredential(header)
	scrubAuthorization(header)
	if headerErr != nil {
		return Principal{}, connectError(connect.CodeUnauthenticated, "authentication required")
	}
	if err := ctx.Err(); err != nil {
		return Principal{}, contextConnectError(err)
	}

	verified, verifyErr := callVerifier(ctx, i.verifier, credential)
	if err := ctx.Err(); err != nil {
		return Principal{}, contextConnectError(err)
	}
	if verifyErr != nil {
		return Principal{}, mapVerificationError(verifyErr)
	}
	if !validVerifiedSession(verified) {
		return Principal{}, connectError(connect.CodeInternal, "session verifier returned invalid claims")
	}

	return Principal{
		tenantID: verified.TenantID,
		actor: ActorRef{
			actorType: verified.ActorType,
			id:        verified.ActorID,
		},
		deviceID:  verified.DeviceID,
		sessionID: verified.SessionID,
	}, nil
}

var errVerifierPanicked = errors.New("session verifier panicked")

func callVerifier(
	ctx context.Context,
	verifier SessionVerifier,
	credential string,
) (verified VerifiedSession, err error) {
	defer func() {
		if recover() != nil {
			verified = VerifiedSession{}
			err = errVerifierPanicked
		}
	}()
	return verifier.VerifySession(ctx, credential)
}

func mapVerificationError(err error) error {
	if errors.Is(err, context.Canceled) {
		return contextConnectError(context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return contextConnectError(context.DeadlineExceeded)
	}
	if errors.Is(err, errVerifierPanicked) {
		return connectError(connect.CodeInternal, "session verifier failed")
	}

	var verificationError *VerificationError
	if errors.As(err, &verificationError) {
		switch verificationError.Failure() {
		case VerificationRejected:
			return connectError(connect.CodeUnauthenticated, "session rejected")
		case VerificationExpired:
			return connectError(connect.CodeUnauthenticated, "session expired")
		case VerificationRevoked:
			return connectError(connect.CodeUnauthenticated, "session revoked")
		default:
			return connectError(connect.CodeInternal, "session verifier returned invalid failure")
		}
	}

	return connectError(connect.CodeUnavailable, "session verification unavailable")
}

func contextConnectError(err error) error {
	if errors.Is(err, context.Canceled) {
		return connectError(connect.CodeCanceled, "request canceled")
	}
	return connectError(connect.CodeDeadlineExceeded, "request deadline exceeded")
}

func connectError(code connect.Code, message string) error {
	return connect.NewError(code, errors.New(message))
}

func validVerifiedSession(verified VerifiedSession) bool {
	if verified.ActorType != ActorTypeHuman &&
		verified.ActorType != ActorTypeAgent &&
		verified.ActorType != ActorTypeService {
		return false
	}
	return validClaim(verified.TenantID) &&
		validClaim(verified.ActorID) &&
		validClaim(verified.DeviceID) &&
		validClaim(verified.SessionID)
}

func validClaim(value string) bool {
	return value != "" && value == strings.TrimSpace(value) &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func readBearerCredential(header http.Header) (string, error) {
	values := authorizationValues(header)
	if len(values) != 1 {
		return "", errors.New("authorization header count")
	}

	value := values[0]
	scheme, credential, found := strings.Cut(value, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || !validToken68(credential) {
		return "", errors.New("malformed authorization header")
	}
	return credential, nil
}

func authorizationValues(header http.Header) []string {
	var values []string
	for key, keyValues := range header {
		if strings.EqualFold(key, "Authorization") {
			values = append(values, keyValues...)
		}
	}
	return values
}

func scrubAuthorization(header http.Header) {
	for key := range header {
		if strings.EqualFold(key, "Authorization") {
			delete(header, key)
		}
	}
}

func validToken68(value string) bool {
	if value == "" {
		return false
	}
	seenBase := false
	padding := false
	for _, character := range value {
		if character == '=' {
			padding = true
			continue
		}
		if padding || !token68Character(character) {
			return false
		}
		seenBase = true
	}
	return seenBase
}

func token68Character(character rune) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' ||
		strings.ContainsRune("-._~+/", character)
}

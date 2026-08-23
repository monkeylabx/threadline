package rpcmiddleware_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const credentialCanary = "Tl9_Jwt.Canary-7uQ5fVx2mR8kZ4pN6wS3aD1gH0jL"

type verifierResult struct {
	verified rpcmiddleware.VerifiedSession
	err      error
}

type inMemoryVerifier struct {
	sessions map[string]verifierResult
	calls    atomic.Int64
}

func (v *inMemoryVerifier) VerifySession(
	_ context.Context,
	credential string,
) (rpcmiddleware.VerifiedSession, error) {
	v.calls.Add(1)
	result, ok := v.sessions[credential]
	if !ok {
		return rpcmiddleware.VerifiedSession{}, rpcmiddleware.NewVerificationError(
			rpcmiddleware.VerificationRejected,
		)
	}
	return result.verified, result.err
}

type verifierFunc func(context.Context, string) (rpcmiddleware.VerifiedSession, error)

func (f verifierFunc) VerifySession(
	ctx context.Context,
	credential string,
) (rpcmiddleware.VerifiedSession, error) {
	return f(ctx, credential)
}

type testMessage struct {
	Index int
	Body  string
}

func validSession(index int) rpcmiddleware.VerifiedSession {
	return rpcmiddleware.VerifiedSession{
		TenantID:  fmt.Sprintf("tenant-%d", index),
		ActorType: rpcmiddleware.ActorTypeHuman,
		ActorID:   fmt.Sprintf("actor-%d", index),
		DeviceID:  fmt.Sprintf("device-%d", index),
		SessionID: fmt.Sprintf("session-%d", index),
	}
}

func unaryRequest(authorizationValues ...string) *connect.Request[testMessage] {
	request := connect.NewRequest(&testMessage{Body: "ordinary request body"})
	for _, value := range authorizationValues {
		request.Header().Add("Authorization", value)
	}
	return request
}

func TestUnarySuccessInjectsOnlyImmutablePrincipalAndPreservesContext(t *testing.T) {
	verified := validSession(1)
	verifier := &inMemoryVerifier{sessions: map[string]verifierResult{
		credentialCanary: {verified: verified},
	}}
	interceptor := rpcmiddleware.NewAuthenticationInterceptor(verifier)
	request := unaryRequest("Bearer " + credentialCanary)

	type contextKey struct{}
	contextValue := &struct{}{}
	deadline := time.Now().Add(time.Minute)
	baseContext, cancel := context.WithDeadline(
		context.WithValue(context.Background(), contextKey{}, contextValue),
		deadline,
	)
	defer cancel()

	handlerCalled := false
	handler := interceptor.WrapUnary(func(
		ctx context.Context,
		request connect.AnyRequest,
	) (connect.AnyResponse, error) {
		handlerCalled = true
		principal, ok := rpcmiddleware.PrincipalFromContext(ctx)
		if !ok {
			t.Fatal("authenticated Principal missing")
		}
		assertPrincipal(t, principal, verified)
		if ctx.Value(contextKey{}) != contextValue {
			t.Fatal("existing context value changed")
		}
		gotDeadline, ok := ctx.Deadline()
		if !ok || !gotDeadline.Equal(deadline) {
			t.Fatal("request deadline changed")
		}
		if ctx.Done() != baseContext.Done() {
			t.Fatal("request cancellation channel changed")
		}
		if hasAuthorization(request.Header()) {
			t.Fatal("handler could retrieve Authorization metadata")
		}

		innerDiagnostic := fmt.Sprintf("%#v %#v %#v", principal, request.Header(), request.Any())
		if strings.Contains(innerDiagnostic, credentialCanary) {
			t.Fatal("inner handler or logger diagnostic exposed bearer credential")
		}
		return connect.NewResponse(&testMessage{Body: "ok"}), nil
	})

	response, err := handler(baseContext, request)
	if err != nil {
		t.Fatal("valid session returned an error")
	}
	if response == nil || !handlerCalled {
		t.Fatal("valid session did not call handler")
	}
	if verifier.calls.Load() != 1 {
		t.Fatal("verifier call count changed")
	}
	if hasAuthorization(request.Header()) {
		t.Fatal("Authorization metadata remained after interception")
	}
}

func TestAuthorizationHeaderFailuresAreFailClosedAndScrubbed(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
	}{
		{name: "missing", header: http.Header{}},
		{name: "duplicated", header: http.Header{"Authorization": {"Bearer first", "Bearer second"}}},
		{
			name: "duplicated mixed case",
			header: http.Header{
				"Authorization": {"Bearer first"},
				"authorization": {"Bearer second"},
			},
		},
		{name: "empty", header: http.Header{"Authorization": {""}}},
		{name: "empty bearer", header: http.Header{"Authorization": {"Bearer "}}},
		{name: "non bearer", header: http.Header{"Authorization": {"Basic value"}}},
		{name: "leading whitespace", header: http.Header{"Authorization": {" Bearer value"}}},
		{name: "extra whitespace", header: http.Header{"Authorization": {"Bearer  value"}}},
		{name: "comma combined", header: http.Header{"Authorization": {"Bearer first,Bearer second"}}},
		{name: "invalid token padding", header: http.Header{"Authorization": {"Bearer ab=c"}}},
		{name: "only token padding", header: http.Header{"Authorization": {"Bearer ="}}},
		{name: "only repeated token padding", header: http.Header{"Authorization": {"Bearer =="}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier := &inMemoryVerifier{sessions: map[string]verifierResult{}}
			interceptor := rpcmiddleware.NewAuthenticationInterceptor(verifier)
			request := connect.NewRequest(&testMessage{})
			clear(request.Header())
			for key, values := range test.header {
				request.Header()[key] = append([]string(nil), values...)
			}
			handlerCalled := false
			handler := interceptor.WrapUnary(func(
				context.Context,
				connect.AnyRequest,
			) (connect.AnyResponse, error) {
				handlerCalled = true
				return connect.NewResponse(&testMessage{}), nil
			})

			_, err := handler(context.Background(), request)
			assertConnectError(t, err, connect.CodeUnauthenticated, "authentication required")
			if handlerCalled {
				t.Fatal("handler ran after authorization header failure")
			}
			if verifier.calls.Load() != 0 {
				t.Fatal("verifier ran after authorization header failure")
			}
			if hasAuthorization(request.Header()) {
				t.Fatal("failed Authorization metadata was not scrubbed")
			}
		})
	}
}

func TestTypedVerificationFailuresAreDeterministicAndFailClosed(t *testing.T) {
	tests := []struct {
		name    string
		failure rpcmiddleware.VerificationFailure
		message string
	}{
		{name: "rejected", failure: rpcmiddleware.VerificationRejected, message: "session rejected"},
		{name: "expired", failure: rpcmiddleware.VerificationExpired, message: "session expired"},
		{name: "revoked", failure: rpcmiddleware.VerificationRevoked, message: "session revoked"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier := &inMemoryVerifier{sessions: map[string]verifierResult{
				credentialCanary: {err: rpcmiddleware.NewVerificationError(test.failure)},
			}}
			assertUnaryFailure(
				t,
				verifier,
				connect.CodeUnauthenticated,
				test.message,
			)
		})
	}
}

func TestInvalidVerifiedClaimsFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*rpcmiddleware.VerifiedSession)
	}{
		{name: "empty tenant", mutate: func(v *rpcmiddleware.VerifiedSession) { v.TenantID = "" }},
		{name: "untrimmed tenant", mutate: func(v *rpcmiddleware.VerifiedSession) { v.TenantID += " " }},
		{name: "empty actor", mutate: func(v *rpcmiddleware.VerifiedSession) { v.ActorID = "" }},
		{name: "untrimmed actor", mutate: func(v *rpcmiddleware.VerifiedSession) { v.ActorID = " actor" }},
		{name: "empty device", mutate: func(v *rpcmiddleware.VerifiedSession) { v.DeviceID = "" }},
		{name: "untrimmed device", mutate: func(v *rpcmiddleware.VerifiedSession) { v.DeviceID += "\t" }},
		{name: "empty session", mutate: func(v *rpcmiddleware.VerifiedSession) { v.SessionID = "" }},
		{name: "untrimmed session", mutate: func(v *rpcmiddleware.VerifiedSession) { v.SessionID = "\ninvalid" }},
		{name: "unsupported actor type", mutate: func(v *rpcmiddleware.VerifiedSession) { v.ActorType = 99 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verified := validSession(1)
			test.mutate(&verified)
			verifier := &inMemoryVerifier{sessions: map[string]verifierResult{
				credentialCanary: {verified: verified},
			}}
			assertUnaryFailure(
				t,
				verifier,
				connect.CodeInternal,
				"session verifier returned invalid claims",
			)
		})
	}
}

func TestVerifierInfrastructureFailureDoesNotLeakWrappedCredential(t *testing.T) {
	verifier := verifierFunc(func(
		context.Context,
		string,
	) (rpcmiddleware.VerifiedSession, error) {
		return rpcmiddleware.VerifiedSession{}, fmt.Errorf(
			"backend input was %s: %w",
			credentialCanary,
			io.ErrUnexpectedEOF,
		)
	})
	assertUnaryFailure(
		t,
		verifier,
		connect.CodeUnavailable,
		"session verification unavailable",
	)
}

func TestInvalidTypedFailureIsInternal(t *testing.T) {
	verifier := &inMemoryVerifier{sessions: map[string]verifierResult{
		credentialCanary: {err: rpcmiddleware.NewVerificationError(99)},
	}}
	assertUnaryFailure(
		t,
		verifier,
		connect.CodeInternal,
		"session verifier returned invalid failure",
	)
}

func TestVerifierPanicIsContainedWithoutLeakingPanicValue(t *testing.T) {
	verifier := verifierFunc(func(
		_ context.Context,
		credential string,
	) (rpcmiddleware.VerifiedSession, error) {
		panic("verifier panic included " + credential)
	})
	assertUnaryFailure(t, verifier, connect.CodeInternal, "session verifier failed")
}

func TestHandlerPanicIsNotRecovered(t *testing.T) {
	verifier := &inMemoryVerifier{sessions: map[string]verifierResult{
		credentialCanary: {verified: validSession(1)},
	}}
	interceptor := rpcmiddleware.NewAuthenticationInterceptor(verifier)
	panicMarker := &struct{}{}
	handler := interceptor.WrapUnary(func(
		context.Context,
		connect.AnyRequest,
	) (connect.AnyResponse, error) {
		panic(panicMarker)
	})

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = handler(context.Background(), unaryRequest("Bearer "+credentialCanary))
	}()
	if recovered != panicMarker {
		t.Fatal("handler panic was swallowed or changed")
	}
}

func TestCanceledAndExpiredContextsKeepConnectSemantics(t *testing.T) {
	tests := []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		code connect.Code
		text string
	}{
		{
			name: "canceled",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() {}
			},
			code: connect.CodeCanceled,
			text: "request canceled",
		},
		{
			name: "deadline exceeded",
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			},
			code: connect.CodeDeadlineExceeded,
			text: "request deadline exceeded",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := test.ctx()
			defer cancel()
			verifier := &inMemoryVerifier{sessions: map[string]verifierResult{
				credentialCanary: {verified: validSession(1)},
			}}
			interceptor := rpcmiddleware.NewAuthenticationInterceptor(verifier)
			handlerCalled := false
			handler := interceptor.WrapUnary(func(
				context.Context,
				connect.AnyRequest,
			) (connect.AnyResponse, error) {
				handlerCalled = true
				return connect.NewResponse(&testMessage{}), nil
			})

			_, err := handler(ctx, unaryRequest("Bearer "+credentialCanary))
			assertConnectError(t, err, test.code, test.text)
			if handlerCalled || verifier.calls.Load() != 0 {
				t.Fatal("canceled request crossed the authentication boundary")
			}
		})
	}
}

func TestWrappedVerifierContextErrorsKeepConnectSemantics(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code connect.Code
		text string
	}{
		{name: "canceled", err: context.Canceled, code: connect.CodeCanceled, text: "request canceled"},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			code: connect.CodeDeadlineExceeded,
			text: "request deadline exceeded",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier := verifierFunc(func(
				context.Context,
				string,
			) (rpcmiddleware.VerifiedSession, error) {
				return rpcmiddleware.VerifiedSession{}, fmt.Errorf(
					"adapter included %s: %w",
					credentialCanary,
					test.err,
				)
			})
			assertUnaryFailure(t, verifier, test.code, test.text)
		})
	}
}

func TestVerifierCannotIgnoreCancellationAndReachHandler(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code connect.Code
		text string
	}{
		{
			name: "canceled during verification",
			err:  context.Canceled,
			code: connect.CodeCanceled,
			text: "request canceled",
		},
		{
			name: "deadline exceeded during verification",
			err:  context.DeadlineExceeded,
			code: connect.CodeDeadlineExceeded,
			text: "request deadline exceeded",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := newSwitchableContext(test.err)
			verifier := verifierFunc(func(
				context.Context,
				string,
			) (rpcmiddleware.VerifiedSession, error) {
				ctx.finish()
				return validSession(1), nil
			})
			interceptor := rpcmiddleware.NewAuthenticationInterceptor(verifier)
			handlerCalled := false
			handler := interceptor.WrapUnary(func(
				context.Context,
				connect.AnyRequest,
			) (connect.AnyResponse, error) {
				handlerCalled = true
				return connect.NewResponse(&testMessage{}), nil
			})

			_, err := handler(ctx, unaryRequest("Bearer "+credentialCanary))
			assertConnectError(t, err, test.code, test.text)
			if handlerCalled {
				t.Fatal("late verifier success reached handler after cancellation")
			}
		})
	}
}

func TestStreamingHandlersAuthenticateByDefault(t *testing.T) {
	verified := validSession(2)
	verifier := &inMemoryVerifier{sessions: map[string]verifierResult{
		credentialCanary: {verified: verified},
	}}
	interceptor := rpcmiddleware.NewAuthenticationInterceptor(verifier)
	connection := &streamingHandlerConnection{header: http.Header{
		"Authorization": {"Bearer " + credentialCanary},
	}}
	handlerCalled := false
	handler := interceptor.WrapStreamingHandler(func(
		ctx context.Context,
		connection connect.StreamingHandlerConn,
	) error {
		handlerCalled = true
		principal, ok := rpcmiddleware.PrincipalFromContext(ctx)
		if !ok {
			t.Fatal("streaming Principal missing")
		}
		assertPrincipal(t, principal, verified)
		if hasAuthorization(connection.RequestHeader()) {
			t.Fatal("streaming handler could retrieve Authorization metadata")
		}
		return nil
	})

	if err := handler(context.Background(), connection); err != nil {
		t.Fatal("valid streaming session returned an error")
	}
	if !handlerCalled {
		t.Fatal("valid streaming session did not call handler")
	}
}

func TestStreamingHeaderFailureDoesNotCallHandler(t *testing.T) {
	verifier := &inMemoryVerifier{sessions: map[string]verifierResult{}}
	interceptor := rpcmiddleware.NewAuthenticationInterceptor(verifier)
	connection := &streamingHandlerConnection{header: http.Header{}}
	handlerCalled := false
	handler := interceptor.WrapStreamingHandler(func(
		context.Context,
		connect.StreamingHandlerConn,
	) error {
		handlerCalled = true
		return nil
	})

	err := handler(context.Background(), connection)
	assertConnectError(t, err, connect.CodeUnauthenticated, "authentication required")
	if handlerCalled || verifier.calls.Load() != 0 {
		t.Fatal("streaming header failure crossed the authentication boundary")
	}
}

func TestConcurrentRequestsDoNotSharePrincipalState(t *testing.T) {
	const requestCount = 128
	sessions := make(map[string]verifierResult, requestCount)
	for index := range requestCount {
		credential := fmt.Sprintf("synthetic-token-%d", index)
		sessions[credential] = verifierResult{verified: validSession(index)}
	}
	verifier := &inMemoryVerifier{sessions: sessions}
	interceptor := rpcmiddleware.NewAuthenticationInterceptor(verifier)
	handler := interceptor.WrapUnary(func(
		ctx context.Context,
		request connect.AnyRequest,
	) (connect.AnyResponse, error) {
		message, ok := request.Any().(*testMessage)
		if !ok {
			return nil, errors.New("unexpected request type")
		}
		principal, ok := rpcmiddleware.PrincipalFromContext(ctx)
		if !ok || !principalMatches(principal, validSession(message.Index)) {
			return nil, errors.New("request observed another Principal")
		}
		if hasAuthorization(request.Header()) {
			return nil, errors.New("request retained bearer metadata")
		}
		return connect.NewResponse(&testMessage{Index: message.Index}), nil
	})

	results := make(chan bool, requestCount)
	for index := range requestCount {
		go func() {
			request := connect.NewRequest(&testMessage{Index: index})
			request.Header().Set("Authorization", fmt.Sprintf("Bearer synthetic-token-%d", index))
			response, err := handler(context.Background(), request)
			if err != nil || response == nil {
				results <- false
				return
			}
			results <- true
		}()
	}
	for range requestCount {
		if !<-results {
			t.Fatal("concurrent request failed")
		}
	}
	if verifier.calls.Load() != requestCount {
		t.Fatal("concurrent verifier call count changed")
	}
}

func TestPrincipalIsAbsentBeforeInterception(t *testing.T) {
	if principal, ok := rpcmiddleware.PrincipalFromContext(context.Background()); ok ||
		principal != (rpcmiddleware.Principal{}) {
		t.Fatal("unauthenticated context exposed a Principal")
	}
}

func assertUnaryFailure(
	t *testing.T,
	verifier rpcmiddleware.SessionVerifier,
	code connect.Code,
	message string,
) {
	t.Helper()
	interceptor := rpcmiddleware.NewAuthenticationInterceptor(verifier)
	request := unaryRequest("Bearer " + credentialCanary)
	handlerCalled := false
	handler := interceptor.WrapUnary(func(
		context.Context,
		connect.AnyRequest,
	) (connect.AnyResponse, error) {
		handlerCalled = true
		return connect.NewResponse(&testMessage{}), nil
	})

	_, err := handler(context.Background(), request)
	assertConnectError(t, err, code, message)
	if handlerCalled {
		t.Fatal("handler ran after authentication failure")
	}
	if hasAuthorization(request.Header()) {
		t.Fatal("Authorization metadata remained after authentication failure")
	}
}

func assertConnectError(t *testing.T, err error, code connect.Code, message string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected Connect error")
	}
	if connect.CodeOf(err) != code {
		t.Fatalf("unexpected Connect code: %v", connect.CodeOf(err))
	}
	expected := code.String() + ": " + message
	if err.Error() != expected {
		t.Fatal("unexpected secret-safe Connect error message")
	}
	if strings.Contains(err.Error(), credentialCanary) {
		t.Fatal("Connect error exposed bearer credential")
	}
}

func assertPrincipal(
	t *testing.T,
	principal rpcmiddleware.Principal,
	verified rpcmiddleware.VerifiedSession,
) {
	t.Helper()
	if !principalMatches(principal, verified) {
		t.Fatal("Principal did not exactly match verified identity")
	}
}

func principalMatches(
	principal rpcmiddleware.Principal,
	verified rpcmiddleware.VerifiedSession,
) bool {
	return principal.TenantID() == verified.TenantID &&
		principal.Actor().Type() == verified.ActorType &&
		principal.Actor().ID() == verified.ActorID &&
		principal.DeviceID() == verified.DeviceID &&
		principal.SessionID() == verified.SessionID
}

func hasAuthorization(header http.Header) bool {
	for key := range header {
		if strings.EqualFold(key, "Authorization") {
			return true
		}
	}
	return false
}

type streamingHandlerConnection struct {
	header http.Header
}

func (*streamingHandlerConnection) Spec() connect.Spec { return connect.Spec{} }

func (*streamingHandlerConnection) Peer() connect.Peer { return connect.Peer{} }

func (*streamingHandlerConnection) Receive(any) error { return io.EOF }

func (c *streamingHandlerConnection) RequestHeader() http.Header { return c.header }

func (*streamingHandlerConnection) Send(any) error { return nil }

func (*streamingHandlerConnection) ResponseHeader() http.Header { return http.Header{} }

func (*streamingHandlerConnection) ResponseTrailer() http.Header { return http.Header{} }

type switchableContext struct {
	done     chan struct{}
	err      error
	finished atomic.Bool
}

func newSwitchableContext(err error) *switchableContext {
	return &switchableContext{done: make(chan struct{}), err: err}
}

func (*switchableContext) Deadline() (time.Time, bool) { return time.Time{}, false }

func (c *switchableContext) Done() <-chan struct{} { return c.done }

func (c *switchableContext) Err() error {
	if c.finished.Load() {
		return c.err
	}
	return nil
}

func (*switchableContext) Value(any) any { return nil }

func (c *switchableContext) finish() {
	c.finished.Store(true)
	close(c.done)
}

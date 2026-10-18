package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (pinger fakePinger) Ping(context.Context) error { return pinger.err }

func TestHealthHandlerReportsPoolState(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		pinger fakePinger
		status int
		body   string
	}{
		"pool answers": {pinger: fakePinger{}, status: http.StatusOK, body: "ok"},
		"pool fails":   {pinger: fakePinger{err: errors.New("down")}, status: http.StatusServiceUnavailable, body: "unavailable\n"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			healthHandler(tc.pinger).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			body, err := io.ReadAll(recorder.Result().Body)
			if err != nil {
				t.Fatal("read health body failed")
			}
			if recorder.Code != tc.status || string(body) != tc.body {
				t.Fatalf("health = %d %q, want %d %q", recorder.Code, body, tc.status, tc.body)
			}
		})
	}
}

package server

import (
	"context"
	"io"
	"net/http"
	"time"
)

// Pinger reports whether the backing PostgreSQL pool answers. *pgxpool.Pool
// satisfies it.
type Pinger interface {
	Ping(context.Context) error
}

const healthTimeout = 2 * time.Second

// healthHandler answers 200 "ok" when the pool answers within healthTimeout
// and 503 "unavailable" otherwise. It reveals no error text.
func healthHandler(pinger Pinger) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), healthTimeout)
		defer cancel()
		if err := pinger.Ping(ctx); err != nil {
			http.Error(writer, "unavailable", http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// A failed body write means the probe client went away. A health probe
		// has nothing further to report, so the result is deliberately dropped.
		_, _ = io.WriteString(writer, "ok")
	})
}

package process

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

func healthHandler(ready func() bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func serveHealth(ctx context.Context, address string, ready func() bool) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return errors.New("worker health listen failed")
	}
	server := &http.Server{Handler: healthHandler(ready), ReadHeaderTimeout: 5 * time.Second}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
	case <-result:
		return errors.New("worker health server failed")
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		return errors.New("worker health shutdown failed")
	}
	if err := <-result; !errors.Is(err, http.ErrServerClosed) {
		return errors.New("worker health server failed")
	}
	return nil
}

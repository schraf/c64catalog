package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// serveDir serves dir over HTTP on localhost:port until ctx is cancelled. A
// port of 0 picks a free port.
func serveDir(ctx context.Context, dir string, port int, logf func(string, ...any)) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("localhost", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return serveListener(ctx, listener, dir, logf)
}

func serveListener(ctx context.Context, listener net.Listener, dir string, logf func(string, ...any)) error {
	files := http.FileServer(http.Dir(dir))
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A re-export shows up on the next reload.
			w.Header().Set("Cache-Control", "no-cache")
			files.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	failed := make(chan error, 1)
	go func() { failed <- server.Serve(listener) }()
	logf("serving %s at http://%s/ (Ctrl-C to stop)", dir, listener.Addr())

	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-failed; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServeDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>home</html>"), 0o644)
	os.MkdirAll(filepath.Join(dir, "data"), 0o755)
	os.WriteFile(filepath.Join(dir, "data", "catalog.json"), []byte(`{"total":1}`), 0o644)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveListener(ctx, listener, dir, t.Logf) }()

	base := "http://" + listener.Addr().String()
	get := func(path string) (*http.Response, string) {
		t.Helper()
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}

	resp, body := get("/")
	if resp.StatusCode != 200 || !strings.Contains(body, "home") {
		t.Fatalf("/ = %d %q", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}

	resp, body = get("/data/catalog.json")
	if resp.StatusCode != 200 || body != `{"total":1}` || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("catalog = %d %q %q", resp.StatusCode, body, resp.Header.Get("Content-Type"))
	}

	if resp, _ := get("/missing"); resp.StatusCode != 404 {
		t.Fatalf("/missing = %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestServeDirPortInUse(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	if err := serveDir(context.Background(), t.TempDir(), port, t.Logf); err == nil {
		t.Fatal("a port that is in use should fail")
	}
}

func TestServeDirRejectsBadPort(t *testing.T) {
	for _, port := range []int{-1, 65536} {
		if err := serveDir(context.Background(), t.TempDir(), port, t.Logf); err == nil {
			t.Errorf("port %d should be rejected", port)
		}
	}
}

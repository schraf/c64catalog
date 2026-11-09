package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testFetcher returns a fetcher whose requests all go to server, whatever host
// they name, with fast retries.
func testFetcher(server *httptest.Server) *fetcher {
	target, _ := url.Parse(server.URL)
	client := server.Client()
	client.Transport = rewriteTransport{base: client.Transport, target: target}
	return &fetcher{client: client, retries: 3, backoff: time.Millisecond}
}

type rewriteTransport struct {
	base   http.RoundTripper
	target *url.URL
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = r.target.Scheme
	clone.URL.Host = r.target.Host
	clone.Host = r.target.Host
	return r.base.RoundTrip(clone)
}

func TestEscapeSegment(t *testing.T) {
	tests := map[string]string{
		"Markt__Technik.d64": "Markt__Technik.d64",
		"A+B & C":            "A%2BB%20%26%20C",
		"it's":               "it%27s",
		"plain-name_1.~":     "plain-name_1.~",
		"Abadía":             "Abad%C3%ADa",
	}
	for input, want := range tests {
		if got := escapeSegment(input); got != want {
			t.Errorf("escapeSegment(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestArchiveURLs(t *testing.T) {
	got := downloadURL("174_1991_Markt_Technik_de", "174_1991_Markt__Technik_de.d64")
	if want := "https://archive.org/download/174_1991_Markt_Technik_de/174_1991_Markt__Technik_de.d64"; got != want {
		t.Fatalf("downloadURL = %q", got)
	}
	if got := downloadURL("id", "Spy vs Spy+.d64"); got != "https://archive.org/download/id/Spy%20vs%20Spy%2B.d64" {
		t.Fatalf("downloadURL escaping = %q", got)
	}
	if got := detailsURL("Beamrider_1984_Activision"); got != "https://archive.org/details/Beamrider_1984_Activision" {
		t.Fatalf("detailsURL = %q", got)
	}
}

func TestFetcherRetriesServerErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("hello"))
	}))
	defer server.Close()

	data, err := testFetcher(server).getBytes(context.Background(), "https://archive.org/x", 100)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" || calls.Load() != 3 {
		t.Fatalf("data=%q calls=%d", data, calls.Load())
	}
}

func TestFetcherDoesNotRetryNotFound(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := testFetcher(server).getBytes(context.Background(), "https://archive.org/x", 100)
	if !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v, want errNotFound", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestFetcherGivesUp(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "busy", http.StatusTooManyRequests)
	}))
	defer server.Close()

	f := testFetcher(server)
	if _, err := f.getBytes(context.Background(), "https://archive.org/x", 100); err == nil {
		t.Fatal("expected an error")
	}
	if want := int32(f.retries + 1); calls.Load() != want {
		t.Fatalf("calls = %d, want %d", calls.Load(), want)
	}
}

func TestFetcherEnforcesSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 50)))
	}))
	defer server.Close()

	if _, err := testFetcher(server).getBytes(context.Background(), "https://archive.org/x", 10); err == nil {
		t.Fatal("expected a size limit error")
	}
}

func TestFetcherStopsWhenCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := testFetcher(server)
	f.backoff = time.Hour
	if _, err := f.getBytes(ctx, "https://archive.org/x", 10); err == nil {
		t.Fatal("expected an error")
	}
}

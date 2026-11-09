package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	archiveDownloadBase = "https://archive.org/download/"
	archiveDetailsBase  = "https://archive.org/details/"

	userAgent = "c64catalog/1.0 (+https://github.com/schraf/c64catalog)"
)

var errNotFound = errors.New("not found")

// permanentError marks a failure that retrying cannot fix.
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// downloadURL builds a URL for a file inside an Internet Archive item.
func downloadURL(identifier, file string) string {
	return archiveDownloadBase + escapeSegment(identifier) + "/" + escapePath(file)
}

// detailsURL is the item's page, which runs the game in an in-browser
// emulator.
func detailsURL(identifier string) string {
	return archiveDetailsBase + escapeSegment(identifier)
}

func escapePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = escapeSegment(part)
	}
	return strings.Join(parts, "/")
}

// escapeSegment percent-encodes everything except unreserved characters.
// url.PathEscape leaves "+" and "&" alone, which some Internet Archive file
// names contain.
func escapeSegment(segment string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(segment); i++ {
		c := segment[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

// fetcher is an HTTP client that retries transient failures.
type fetcher struct {
	client  *http.Client
	retries int
	backoff time.Duration
}

func newFetcher() *fetcher {
	return &fetcher{
		client:  &http.Client{Timeout: 15 * time.Minute},
		retries: 4,
		backoff: time.Second,
	}
}

// do issues a GET (with an optional Range header) and passes a successful
// response to handle. Network errors, 429, and 5xx responses are retried.
func (f *fetcher) do(ctx context.Context, rawURL, byteRange string, handle func(*http.Response) error) error {
	var lastErr error
	for attempt := 0; attempt <= f.retries; attempt++ {
		if attempt > 0 {
			delay := f.backoff << (attempt - 1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := f.attempt(ctx, rawURL, byteRange, handle)
		if err == nil {
			return nil
		}
		var permanent permanentError
		if errors.As(err, &permanent) || ctx.Err() != nil {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("%s: %w", rawURL, lastErr)
}

func (f *fetcher) attempt(ctx context.Context, rawURL, byteRange string, handle func(*http.Response) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return permanentError{err}
	}
	req.Header.Set("User-Agent", userAgent)
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent:
		return handle(resp)
	case resp.StatusCode == http.StatusNotFound:
		return permanentError{fmt.Errorf("%s: %w", rawURL, errNotFound)}
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return fmt.Errorf("status %s", resp.Status)
	default:
		return permanentError{fmt.Errorf("%s: status %s", rawURL, resp.Status)}
	}
}

// getBytes downloads a URL into memory, refusing bodies larger than limit.
func (f *fetcher) getBytes(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	var data []byte
	err := f.do(ctx, rawURL, "", func(resp *http.Response) error {
		body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		if err != nil {
			return err
		}
		if int64(len(body)) > limit {
			return permanentError{fmt.Errorf("%s: larger than %d bytes", rawURL, limit)}
		}
		data = body
		return nil
	})
	return data, err
}

// download streams a URL into a new temporary file in dir and returns its path.
func (f *fetcher) download(ctx context.Context, rawURL, dir, pattern string) (string, error) {
	var name string
	err := f.do(ctx, rawURL, "", func(resp *http.Response) error {
		file, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return permanentError{err}
		}
		_, copyErr := io.Copy(file, resp.Body)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(file.Name())
			return errors.Join(copyErr, closeErr)
		}
		name = file.Name()
		return nil
	})
	return name, err
}

package controllerhttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDoUnexpectedStatusReturnsBoundedSanitizedErrorAndClosesBody(t *testing.T) {
	token, err := NewSensitiveToken("secret-sentinel")
	if err != nil {
		t.Fatalf("NewSensitiveToken() error = %v", err)
	}
	body := &trackingReadCloser{reader: strings.NewReader(strings.Repeat("x", ErrorBodyLimit+100) + "\x00secret-sentinel")}
	client, err := New(Config{
		BaseURL: "http://localhost:8080",
		HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       body,
				Header:     http.Header{},
			}, nil
		})},
		Token: NewStaticTokenProvider(token),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request, err := client.NewRequest(context.Background(), http.MethodGet, "/status", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	response, err := client.Do(request, http.StatusOK)
	if err == nil {
		if response != nil {
			response.Body.Close()
		}
		t.Fatal("expected status error")
	}
	if !body.closed {
		t.Fatal("unexpected-status body was not closed")
	}
	var statusErr StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error %T does not wrap StatusError", err)
	}
	if statusErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status code = %d", statusErr.StatusCode)
	}
	if len(statusErr.Body) > ErrorBodyLimit {
		t.Fatalf("body length = %d, want <= %d", len(statusErr.Body), ErrorBodyLimit)
	}
	if strings.Contains(statusErr.Body, "\x00") {
		t.Fatalf("body was not sanitized: %q", statusErr.Body)
	}
	if strings.Contains(err.Error(), "secret-sentinel") {
		t.Fatalf("error leaked token: %q", err.Error())
	}
}

func TestDoExpectedStatusLeavesBodyOpen(t *testing.T) {
	body := &trackingReadCloser{reader: strings.NewReader("ok")}
	client, err := New(Config{
		BaseURL: "http://localhost:8080",
		HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       body,
				Header:     http.Header{},
			}, nil
		})},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request, err := client.NewPublicRequest(context.Background(), http.MethodGet, "/healthz", nil)
	if err != nil {
		t.Fatalf("NewPublicRequest() error = %v", err)
	}

	response, err := client.Do(request, http.StatusNoContent)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if body.closed {
		t.Fatal("expected-status body was closed before caller received response")
	}
	response.Body.Close()
	if !body.closed {
		t.Fatal("caller close did not close response body")
	}
}

type trackingReadCloser struct {
	reader io.Reader
	closed bool
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

package controllerhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultTimeout = 30 * time.Second

type Config struct {
	BaseURL                   string
	HTTP                      *http.Client
	Token                     TokenProvider
	Caller                    string
	AllowInsecureExternalHTTP bool
}

type Client struct {
	baseURL *url.URL
	http    *http.Client
	token   TokenProvider
	caller  string
}

func New(config Config) (Client, error) {
	baseURL, err := parseBaseURL(config.BaseURL, config.AllowInsecureExternalHTTP)
	if err != nil {
		return Client{}, err
	}
	return Client{
		baseURL: baseURL,
		http:    cloneHTTPClient(config.HTTP, baseURL),
		token:   config.Token,
		caller:  callerUserAgent(config.Caller),
	}, nil
}

func (c Client) NewRequest(ctx context.Context, method string, path string, body io.Reader) (*http.Request, error) {
	return c.newRequest(ctx, method, path, nil, body, false, "")
}

func (c Client) NewRequestWithQuery(ctx context.Context, method string, path string, query url.Values, body io.Reader) (*http.Request, error) {
	return c.newRequest(ctx, method, path, query, body, false, "")
}

func (c Client) NewJSONRequest(ctx context.Context, method string, path string, value any) (*http.Request, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode controller request body: %w", err)
	}
	return c.newRequest(ctx, method, path, nil, bytes.NewReader(body), false, "application/json")
}

func (c Client) NewPublicRequest(ctx context.Context, method string, path string, body io.Reader) (*http.Request, error) {
	return c.newRequest(ctx, method, path, nil, body, true, "")
}

func (c Client) Do(request *http.Request, expectedStatus ...int) (*http.Response, error) {
	if c.http == nil {
		return nil, fmt.Errorf("controller http client is not initialized")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("controller http: %w", err)
	}
	if statusExpected(response.StatusCode, expectedStatus) {
		return response, nil
	}
	return nil, newStatusError(request, response)
}

func PathJoin(base string, segments ...string) (string, error) {
	if err := validateRequestPath(base); err != nil {
		return "", err
	}
	parts := []string{strings.TrimRight(base, "/")}
	for _, segment := range segments {
		escaped, err := escapedPathSegment(segment)
		if err != nil {
			return "", err
		}
		parts = append(parts, escaped)
	}
	return strings.Join(parts, "/"), nil
}

func (c Client) newRequest(ctx context.Context, method string, path string, query url.Values, body io.Reader, public bool, contentType string) (*http.Request, error) {
	if c.baseURL == nil {
		return nil, fmt.Errorf("controller http client is not initialized")
	}
	if err := validateRequestPath(path); err != nil {
		return nil, err
	}
	requestURL, err := c.requestURL(path, query)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create controller request: %w", err)
	}
	request.Header.Set("User-Agent", c.caller)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if public {
		return request, nil
	}
	if c.token == nil {
		return nil, fmt.Errorf("controller token provider is required")
	}
	token, err := c.token.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("controller token: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token.Plaintext())
	return request, nil
}

func (c Client) requestURL(path string, query url.Values) (*url.URL, error) {
	target := *c.baseURL
	escapedPath := strings.TrimRight(target.EscapedPath(), "/") + path
	decodedPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return nil, fmt.Errorf("controller request path is invalid: %w", err)
	}
	target.Path = decodedPath
	target.RawPath = escapedPath
	if len(query) != 0 {
		target.RawQuery = query.Encode()
	}
	return &target, nil
}

func parseBaseURL(raw string, allowInsecureExternalHTTP bool) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("controller base URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("controller base URL is invalid: %w", err)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("controller base URL must not contain user info")
	}
	if parsed.Fragment != "" {
		return nil, fmt.Errorf("controller base URL must not contain a fragment")
	}
	if parsed.RawQuery != "" {
		return nil, fmt.Errorf("controller base URL must not contain a query string")
	}
	if parsed.Scheme == "" {
		return nil, fmt.Errorf("controller base URL requires a scheme")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("controller base URL requires a host")
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if !allowInsecureExternalHTTP && !isLoopbackHost(parsed.Hostname()) {
			return nil, fmt.Errorf("controller base URL %q uses plain HTTP with a non-loopback host", raw)
		}
	default:
		return nil, fmt.Errorf("controller base URL scheme %q is unsupported", parsed.Scheme)
	}
	return parsed, nil
}

func validateRequestPath(path string) error {
	switch {
	case path == "":
		return fmt.Errorf("controller request path is required")
	case !strings.HasPrefix(path, "/"):
		return fmt.Errorf("controller request path must start with /")
	case strings.HasPrefix(path, "//"):
		return fmt.Errorf("controller request path must not start with //")
	case strings.Contains(path, "?"):
		return fmt.Errorf("controller request path must not contain a query string")
	case strings.Contains(path, "#"):
		return fmt.Errorf("controller request path must not contain a fragment")
	case strings.Contains(path, `\`):
		return fmt.Errorf("controller request path must not contain backslashes")
	}
	if parsed, err := url.Parse(path); err != nil || parsed.IsAbs() || parsed.Host != "" {
		return fmt.Errorf("controller request path must not contain a scheme or host")
	}
	return nil
}

func escapedPathSegment(segment string) (string, error) {
	if segment == "" {
		return "", fmt.Errorf("controller route segment is required")
	}
	if strings.ContainsAny(segment, `/\`) {
		return "", fmt.Errorf("controller route segment must not contain path separators")
	}
	decoded, err := url.PathUnescape(segment)
	if err != nil {
		return "", fmt.Errorf("controller route segment is invalid: %w", err)
	}
	if decoded == "" || strings.ContainsAny(decoded, `/\`) {
		return "", fmt.Errorf("controller route segment must not decode to path separators")
	}
	return url.PathEscape(segment), nil
}

func cloneHTTPClient(input *http.Client, baseURL *url.URL) *http.Client {
	var cloned http.Client
	if input != nil {
		cloned = *input
	}
	if cloned.Timeout == 0 {
		cloned.Timeout = DefaultTimeout
	}
	cloned.CheckRedirect = sameOriginRedirect(baseURL)
	return &cloned
}

func sameOriginRedirect(baseURL *url.URL) func(*http.Request, []*http.Request) error {
	return func(request *http.Request, prior []*http.Request) error {
		if request.URL.Scheme != baseURL.Scheme || request.URL.Host != baseURL.Host {
			return http.ErrUseLastResponse
		}
		return nil
	}
}

func statusExpected(statusCode int, expected []int) bool {
	if len(expected) == 0 {
		return statusCode >= 200 && statusCode < 300
	}
	for _, candidate := range expected {
		if statusCode == candidate {
			return true
		}
	}
	return false
}

func callerUserAgent(caller string) string {
	if strings.TrimSpace(caller) == "" {
		return "goetl-controller-http/1"
	}
	return strings.TrimSpace(caller)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

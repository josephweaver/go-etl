package controllerhttp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"
)

const ErrorBodyLimit = 4 * 1024

type StatusError struct {
	StatusCode int
	Body       string
}

func (e StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("controller http: unexpected status %d", e.StatusCode)
	}
	return fmt.Sprintf("controller http: unexpected status %d: %s", e.StatusCode, e.Body)
}

func newStatusError(request *http.Request, response *http.Response) error {
	defer response.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(response.Body, ErrorBodyLimit+1))
	if len(body) > ErrorBodyLimit {
		body = body[:ErrorBodyLimit]
	}

	text := sanitizeErrorBody(body)
	if token := bearerTokenFromRequest(request); token != "" {
		text = strings.ReplaceAll(text, token, tokenRedactionLabel)
	}
	return StatusError{
		StatusCode: response.StatusCode,
		Body:       text,
	}
}

func bearerTokenFromRequest(request *http.Request) string {
	if request == nil {
		return ""
	}
	const prefix = "Bearer "
	value := request.Header.Get("Authorization")
	if !strings.HasPrefix(value, prefix) {
		return ""
	}
	return strings.TrimPrefix(value, prefix)
}

func sanitizeErrorBody(body []byte) string {
	text := strings.ToValidUTF8(string(bytes.TrimSpace(body)), "")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, text)
}

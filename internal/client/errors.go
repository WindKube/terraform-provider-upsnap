package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type FieldError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIError is a non-2xx response from UpSnap.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	Message    string
	Fields     map[string]FieldError
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: HTTP %d", e.Method, e.Path, e.StatusCode)
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	if len(e.Fields) > 0 {
		names := make([]string, 0, len(e.Fields))
		for name := range e.Fields {
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, name := range names {
			parts = append(parts, fmt.Sprintf("%s: %s", name, e.Fields[name].Message))
		}
		fmt.Fprintf(&b, " (%s)", strings.Join(parts, "; "))
	}
	return b.String()
}

type AuthError struct {
	Message string
	Err     error
}

func (e *AuthError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *AuthError) Unwrap() error { return e.Err }

func newAPIError(method, path string, status int, body []byte) *APIError {
	apiErr := &APIError{Method: method, Path: path, StatusCode: status}
	var parsed struct {
		Message string                     `json:"message"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		apiErr.Message = parsed.Message
		for name, raw := range parsed.Data {
			var fe FieldError
			if json.Unmarshal(raw, &fe) == nil && fe.Message != "" {
				if apiErr.Fields == nil {
					apiErr.Fields = map[string]FieldError{}
				}
				apiErr.Fields[name] = fe
			}
		}
	}
	if apiErr.Message == "" {
		apiErr.Message = strings.TrimSpace(string(body))
		if len(apiErr.Message) > 512 {
			apiErr.Message = apiErr.Message[:512]
		}
	}
	return apiErr
}

// IsNotFound reports whether err is a 404 from UpSnap. PocketBase also answers 404 when
// a record exists but the caller's API rules hide it.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func IsForbidden(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusForbidden || apiErr.StatusCode == http.StatusUnauthorized)
}

package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

// apiBaseURL returns the base URL for the API from environment or default.
func apiBaseURL() string {
	if url := os.Getenv("API_URL"); url != "" {
		return url
	}
	return "http://localhost:8080"
}

// TestClient wraps an HTTP client with authentication headers and helper methods.
type TestClient struct {
	*http.Client
	BaseURL   string
	UserID    string
	TeamID    string
	UserRoles string
	RequestID string
}

// NewTestClient creates a new test client with default authentication headers.
// By default, creates a client with admin role for full access.
func NewTestClient(t *testing.T) *TestClient {
	t.Helper()
	return &TestClient{
		Client:    &http.Client{Timeout: 10 * time.Second},
		BaseURL:   apiBaseURL(),
		UserID:    uuid.New().String(),
		TeamID:    uuid.New().String(),
		UserRoles: "admin,developer",
		RequestID: uuid.New().String(),
	}
}

// NewReadOnlyTestClient creates a client without admin role (developer only).
func NewReadOnlyTestClient(t *testing.T) *TestClient {
	t.Helper()
	client := NewTestClient(t)
	client.UserRoles = "developer"
	return client
}

// NewUnauthenticatedClient creates a client without authentication headers.
func NewUnauthenticatedClient(t *testing.T) *TestClient {
	t.Helper()
	return &TestClient{
		Client:  &http.Client{Timeout: 10 * time.Second},
		BaseURL: apiBaseURL(),
	}
}

// Do executes an HTTP request with authentication headers.
func (c *TestClient) Do(req *http.Request) (*http.Response, error) {
	if c.UserID != "" {
		req.Header.Set("X-User-ID", c.UserID)
	}
	if c.TeamID != "" {
		req.Header.Set("X-Team-ID", c.TeamID)
	}
	if c.UserRoles != "" {
		req.Header.Set("X-User-Roles", c.UserRoles)
	}
	if c.RequestID != "" {
		req.Header.Set("X-Request-ID", c.RequestID)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.Client.Do(req)
}

// Get performs a GET request to the specified path.
func (c *TestClient) Get(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post performs a POST request with JSON body.
func (c *TestClient) Post(path string, body any) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Put performs a PUT request with JSON body.
func (c *TestClient) Put(path string, body any) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPut, c.BaseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Delete performs a DELETE request.
func (c *TestClient) Delete(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// ReadBody reads and closes the response body.
func ReadBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	return body
}

// ParseJSON unmarshals JSON response body into the provided struct.
func ParseJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var result T
	body := ReadBody(t, resp)
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v\nBody: %s", err, string(body))
	}
	return result
}

// AssertStatusCode verifies the response status code matches expected.
func AssertStatusCode(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d\nBody: %s", expected, resp.StatusCode, string(body))
	}
}

// AssertContentType verifies the Content-Type header.
func AssertContentType(t *testing.T, resp *http.Response, expected string) {
	t.Helper()
	ct := resp.Header.Get("Content-Type")
	if ct != expected {
		t.Fatalf("expected Content-Type %q, got %q", expected, ct)
	}
}

// AssertRequestIDHeader verifies X-Request-ID header is present.
func AssertRequestIDHeader(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp.Header.Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID header to be present")
	}
}

// GenerateUniqueName creates a unique name for test resources.
func GenerateUniqueName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8])
}

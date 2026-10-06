package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const defaultBaseURL = "https://api.openai.com/v1"

var errMissingAdminAPIKey = errors.New("no OpenAI Admin API key is configured. Set the provider's admin_api_key attribute or the OPENAI_ADMIN_KEY environment variable to an Admin API key created in the OpenAI organization settings")

type clientConfig struct {
	adminAPIKey string
	baseURL     string
}

type adminClient struct {
	httpClient *http.Client
	baseURL    string
}

// newClient resolves each setting from the configuration first, then from
// OPENAI_ADMIN_KEY and OPENAI_BASE_URL, so an explicit attribute always wins
// over the environment.
func newClient(cfg clientConfig) (*adminClient, error) {
	adminAPIKey := cfg.adminAPIKey
	if adminAPIKey == "" {
		adminAPIKey = os.Getenv("OPENAI_ADMIN_KEY")
	}
	if adminAPIKey == "" {
		return nil, errMissingAdminAPIKey
	}
	baseURL := cfg.baseURL
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_BASE_URL")
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &adminClient{
		httpClient: newRetryableClient(bearerTokenTransport{token: adminAPIKey, next: http.DefaultTransport}),
		baseURL:    strings.TrimSuffix(baseURL, "/"),
	}, nil
}

type bearerTokenTransport struct {
	token string
	next  http.RoundTripper
}

func (t bearerTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	authenticated := req.Clone(req.Context())
	authenticated.Header.Set("Authorization", "Bearer "+t.token)
	return t.next.RoundTrip(authenticated)
}

type apiError struct {
	statusCode int
	errorType  string
	message    string
}

func (e *apiError) Error() string {
	if e.message == "" {
		return fmt.Sprintf("OpenAI API returned HTTP %d", e.statusCode)
	}
	if e.errorType == "" {
		return fmt.Sprintf("OpenAI API returned HTTP %d: %s", e.statusCode, e.message)
	}
	return fmt.Sprintf("OpenAI API returned HTTP %d (%s): %s", e.statusCode, e.errorType, e.message)
}

func (c *adminClient) do(ctx context.Context, method, path string, requestBody, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("unable to encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("unable to build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return decodeAPIError(response)
	}
	if responseBody == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(responseBody); err != nil {
		return fmt.Errorf("unable to decode response: %w", err)
	}
	return nil
}

func decodeAPIError(response *http.Response) error {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	_ = json.NewDecoder(response.Body).Decode(&envelope)
	return &apiError{
		statusCode: response.StatusCode,
		errorType:  envelope.Error.Type,
		message:    envelope.Error.Message,
	}
}

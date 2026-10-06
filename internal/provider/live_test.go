package provider

import (
	"net/http"
	"os"
	"testing"
)

// requireLiveEndpoint is the starting point for live acceptance tests against
// a real API, skipped unless TF_ACC and EXAMPLE_TEST_ENDPOINT are set. Replace
// it once the provider has a resource or data source to exercise.
func requireLiveEndpoint(t *testing.T) string {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run live acceptance tests against a real API")
	}
	endpoint := os.Getenv("EXAMPLE_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("set EXAMPLE_TEST_ENDPOINT to run live acceptance tests against a real API")
	}
	return endpoint
}

func TestLive_Placeholder(t *testing.T) {
	endpoint := requireLiveEndpoint(t)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	resp, err := newRetryableClient(nil).Do(req)
	if err != nil {
		t.Fatalf("expected a live request to succeed, got: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
}

package provider

import (
	"context"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const requestAttemptTimeout = 90 * time.Second

type requestMethodKey struct{}

type methodTaggingTransport struct {
	next http.RoundTripper
}

func (t methodTaggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.next.RoundTrip(req.WithContext(context.WithValue(req.Context(), requestMethodKey{}, req.Method)))
}

// newRetryableClient wraps base in a retryablehttp transport, bounding every
// attempt at requestAttemptTimeout. When base is nil it falls back to
// http.DefaultTransport.
func newRetryableClient(base http.RoundTripper) *http.Client {
	if base == nil {
		base = http.DefaultTransport
	}
	client := retryablehttp.NewClient()
	client.HTTPClient = &http.Client{
		Timeout:   requestAttemptTimeout,
		Transport: base,
	}
	client.RetryMax = 5
	client.CheckRetry = retryPolicy
	client.ErrorHandler = retryablehttp.PassthroughErrorHandler
	client.Logger = nil
	return &http.Client{Transport: methodTaggingTransport{next: &retryablehttp.RoundTripper{Client: client}}}
}

func retryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if method, _ := ctx.Value(requestMethodKey{}).(string); method == http.MethodPost {
		return false, nil
	}
	if err != nil {
		return true, nil
	}
	if resp.StatusCode == 429 {
		return true, nil
	}
	if resp.StatusCode >= 500 && resp.StatusCode != 501 {
		return true, nil
	}
	return false, nil
}

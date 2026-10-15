package testutil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestProviderResponseModes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		retry  string
	}{
		{"success", 200, `{"status_code":200,"data":[]}`, ""},
		{"authentication", 401, `{"error":"unauthorized"}`, ""},
		{"rate-limit", 429, `{"error":"rate limited"}`, "3"},
		{"unavailable", 503, `{"error":"unavailable"}`, ""},
		{"malformed", 200, `{`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewProviderServer(t, ProviderResponse{StatusCode: tc.status, Body: []byte(tc.body), Headers: http.Header{"Retry-After": {tc.retry}}})
			response, err := provider.Server.Client().Get(provider.Server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != tc.status || string(body) != tc.body || response.Header.Get("Retry-After") != tc.retry || provider.Calls() != 1 {
				t.Fatalf("response = %d %q, retry %q, calls %d, err %v", response.StatusCode, body, response.Header.Get("Retry-After"), provider.Calls(), err)
			}
		})
	}
}

func TestProviderCancellationAndDeadline(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancellation", true: "deadline"}[deadline], func(t *testing.T) {
			provider := NewProviderServer(t, ProviderResponse{Wait: make(chan struct{})})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if deadline {
				var cancelDeadline context.CancelFunc
				ctx, cancelDeadline = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancelDeadline()
			}
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.Server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				response, err := provider.Server.Client().Do(request)
				if response != nil {
					response.Body.Close()
				}
				result <- err
			}()
			select {
			case <-provider.Started:
			case <-time.After(2 * time.Second):
				t.Fatal("request did not reach local provider")
			}
			want := context.DeadlineExceeded
			if !deadline {
				cancel()
				want = context.Canceled
			}
			if err := <-result; !errors.Is(err, want) {
				t.Fatalf("error = %v, want %v", err, want)
			}
			select {
			case <-provider.Canceled:
			case <-time.After(2 * time.Second):
				t.Fatal("provider did not observe cancellation")
			}
		})
	}
}

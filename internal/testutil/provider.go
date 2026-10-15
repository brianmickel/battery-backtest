package testutil

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// ProviderResponse describes one deterministic upstream response. A non-nil Wait
// holds the response until released or the incoming request is canceled.
type ProviderResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
	Wait       <-chan struct{}
}

type ProviderServer struct {
	Server   *httptest.Server
	Started  <-chan struct{}
	Canceled <-chan struct{}
	calls    atomic.Int64
}

func NewProviderServer(t testing.TB, response ProviderResponse) *ProviderServer {
	t.Helper()
	started := make(chan struct{}, 32)
	canceled := make(chan struct{}, 32)
	provider := &ProviderServer{Started: started, Canceled: canceled}
	provider.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider.calls.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		if response.Wait != nil {
			select {
			case <-response.Wait:
			case <-r.Context().Done():
				select {
				case canceled <- struct{}{}:
				default:
				}
				return
			}
		}
		for name, values := range response.Headers {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		status := response.StatusCode
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write(response.Body)
	}))
	t.Cleanup(provider.Server.Close)
	return provider
}

func (p *ProviderServer) Calls() int {
	return int(p.calls.Load())
}

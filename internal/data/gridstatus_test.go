package data

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"battery-backtest/internal/model"
)

type providerTransport func(*http.Request) (*http.Response, error)

func (f providerTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func providerClient(t *testing.T, client *http.Client, baseURL string) *GridStatusClient {
	t.Helper()
	provider, err := NewGridStatusClient(client, baseURL)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func providerQuery() Query {
	return Query{DatasetID: "test-dataset", LocationID: "test-node", StartDate: "2024-01-01", EndDate: "2024-01-02", Clock: "market"}
}

func providerRow() model.LMPInterval {
	start, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00-08:00")
	return model.LMPInterval{
		IntervalStartUTC: start.UTC(), IntervalEndUTC: start.Add(5 * time.Minute).UTC(),
		IntervalStartLocal: start, IntervalEndLocal: start.Add(5 * time.Minute),
		Market: "RT", Location: "test-node", LocationType: "Trading Hub", LMP: -20,
	}
}

func providerPayload(t *testing.T, rows ...model.LMPInterval) string {
	t.Helper()
	if rows == nil {
		rows = []model.LMPInterval{}
	}
	body, err := json.Marshal(model.GridStatusLMPResponse{StatusCode: 200, Data: rows})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func providerResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}

func TestProviderRequestEncodingAndClock(t *testing.T) {
	for _, clock := range []string{"", "market", "UTC"} {
		for _, id := range []string{"a/b?x=y#frag&colon: space%", ".", "..", "//other.example/path"} {
			t.Run(clock+"/"+id, func(t *testing.T) {
				query := providerQuery()
				query.DatasetID, query.LocationID, query.Clock = id, id, clock
				row := providerRow()
				row.Location = id
				payload := providerPayload(t, row)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					wantPath := "/prefix/v1/datasets/" + url.PathEscape(id) + "/query/location/" + url.PathEscape(id)
					if id == "." || id == ".." {
						wantPath = "/prefix/v1/datasets/" + strings.ReplaceAll(id, ".", "%2E") + "/query/location/" + strings.ReplaceAll(id, ".", "%2E")
					}
					if req.URL.EscapedPath() != wantPath {
						t.Errorf("path = %q, want %q", req.URL.EscapedPath(), wantPath)
					}
					wantClock := clock
					if wantClock == "" {
						wantClock = "market"
					}
					wantQuery := url.Values{"start_time": {query.StartDate}, "end_time": {query.EndDate}, "timezone": {wantClock}, "download": {"true"}}
					if !reflect.DeepEqual(req.URL.Query(), wantQuery) || req.URL.Fragment != "" {
						t.Errorf("query = %v, want %v", req.URL.Query(), wantQuery)
					}
					if req.Method != "GET" || req.Header.Get("x-api-key") != "k" || req.Header.Get("Accept") != "application/json" {
						t.Error("method or request headers did not match")
					}
					_, _ = io.WriteString(w, payload)
				}))
				defer server.Close()
				rows, err := providerClient(t, server.Client(), server.URL+"/prefix/").Fetch(context.Background(), "k", query)
				if err != nil || len(rows) != 1 || rows[0].Location != id || rows[0].LMP != -20 {
					t.Fatalf("rows = %#v, err = %v", rows, err)
				}
				_, offset := rows[0].IntervalStartLocal.Zone()
				if (clock == "UTC" && offset != 0) || (clock != "UTC" && offset != -8*60*60) {
					t.Errorf("clock %q returned offset %d", clock, offset)
				}
			})
		}
	}
}

func TestProviderConstructorRejectsUnsafeBaseURL(t *testing.T) {
	for _, input := range []string{"relative", "ftp://example.com", "https://user:secret@example.com", "https://example.com?secret=value", "https://example.com?", "https://example.com/#fragment", "https://%", "https://"} {
		if client, err := NewGridStatusClient(nil, input); err == nil || client != nil {
			t.Errorf("base URL %q accepted", input)
		} else if strings.Contains(err.Error(), "secret") {
			t.Errorf("base URL leaked in error: %v", err)
		}
	}
	client := providerClient(t, nil, "")
	if client.baseURL.String() != "https://api.gridstatus.io" || http.DefaultClient.CheckRedirect != nil {
		t.Fatal("default constructor changed default client or base URL")
	}
}

func TestProviderRejectsAllRedirectsAndPreservesInjectedClient(t *testing.T) {
	for _, status := range []int{300, 301, 302, 303, 304, 305, 307, 308, 399} {
		for _, destination := range []string{"https://elsewhere.example/private", "/same-origin"} {
			t.Run(fmt.Sprintf("%d/%s", status, destination), func(t *testing.T) {
				var calls, originalPolicyCalls atomic.Int32
				transport := providerTransport(func(req *http.Request) (*http.Response, error) {
					calls.Add(1)
					if req.URL.Host != "original.example" || req.URL.EscapedPath() != "/v1/datasets/test-dataset/query/location/test-node" {
						t.Error("credential-bearing request escaped its original endpoint")
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Location": {destination}}, Body: io.NopCloser(strings.NewReader("dummy-secret"))}, nil
				})
				jar, _ := cookiejar.New(nil)
				redirect := func(*http.Request, []*http.Request) error { originalPolicyCalls.Add(1); return nil }
				injected := &http.Client{Transport: transport, Jar: jar, Timeout: 2 * time.Minute, CheckRedirect: redirect}
				provider := providerClient(t, injected, "https://original.example")
				_, err := provider.Fetch(context.Background(), "dummy-secret", providerQuery())
				var upstream *GridStatusError
				if !errors.As(fmt.Errorf("outer: %w", err), &upstream) || upstream.StatusCode != status || upstream.Code != "UPSTREAM_UNAVAILABLE" {
					t.Fatalf("redirect error = %#v", err)
				}
				if calls.Load() != 1 || originalPolicyCalls.Load() != 0 {
					t.Errorf("requests = %d, injected policy calls = %d", calls.Load(), originalPolicyCalls.Load())
				}
				if injected.Timeout != 2*time.Minute || injected.Jar != jar || reflect.ValueOf(injected.Transport).Pointer() != reflect.ValueOf(transport).Pointer() || reflect.ValueOf(injected.CheckRedirect).Pointer() != reflect.ValueOf(redirect).Pointer() {
					t.Error("injected client changed")
				}
				if provider.client.Timeout != injected.Timeout || provider.client.Jar != jar || reflect.ValueOf(provider.client.Transport).Pointer() != reflect.ValueOf(transport).Pointer() {
					t.Error("owned copy discarded client configuration")
				}
			})
		}
	}
}

func TestProviderRealRedirectCannotLeakHeader(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		destinationCalls.Add(1)
		if req.Header.Get("x-api-key") != "" {
			t.Error("redirect destination received credential")
		}
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, destination.URL, http.StatusFound)
	}))
	defer origin.Close()
	_, err := providerClient(t, origin.Client(), origin.URL).Fetch(context.Background(), "dummy-secret", providerQuery())
	var upstream *GridStatusError
	if !errors.As(err, &upstream) || upstream.StatusCode != 302 || destinationCalls.Load() != 0 {
		t.Fatalf("err = %v, destination calls = %d", err, destinationCalls.Load())
	}
}

func TestProviderStatusErrorsAreSafeAndTyped(t *testing.T) {
	var logs bytes.Buffer
	originalLog := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(originalLog)
	for _, test := range []struct {
		status int
		code   string
	}{{401, "UPSTREAM_AUTH"}, {403, "UPSTREAM_AUTH"}, {429, "UPSTREAM_RATE_LIMIT"}, {500, "UPSTREAM_UNAVAILABLE"}, {503, "UPSTREAM_UNAVAILABLE"}, {404, "UPSTREAM_UNAVAILABLE"}} {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, "dummy-secret request dump")
			}))
			defer server.Close()
			_, err := providerClient(t, server.Client(), server.URL).Fetch(context.Background(), "dummy-secret", providerQuery())
			var upstream *GridStatusError
			if !errors.As(fmt.Errorf("outer: %w", err), &upstream) || upstream.StatusCode != test.status || upstream.Code != test.code {
				t.Fatalf("error = %#v", err)
			}
			if test.status == 429 && upstream.RetryAfter != "120" {
				t.Errorf("Retry-After = %q", upstream.RetryAfter)
			}
			if strings.Contains(err.Error(), "dummy-secret") || strings.Contains(err.Error(), "request dump") {
				t.Errorf("unsafe error: %v", err)
			}
		})
	}
	if strings.Contains(logs.String(), "dummy-secret") {
		t.Fatal("credential leaked to logger")
	}
}

func TestProviderTransportErrorKeepsCauseWithoutDisclosure(t *testing.T) {
	cause := errors.New("transport included dummy-secret and a request dump")
	transport := providerTransport(func(*http.Request) (*http.Response, error) { return nil, cause })
	_, err := providerClient(t, &http.Client{Transport: transport}, "https://provider.example").Fetch(context.Background(), "dummy-secret", providerQuery())
	var upstream *GridStatusError
	var requestError *url.Error
	if !errors.As(err, &upstream) || upstream.Code != "UPSTREAM_UNAVAILABLE" || !errors.Is(err, cause) || !errors.As(err, &requestError) {
		t.Fatalf("transport error identities lost: %v", err)
	}
	if strings.Contains(fmt.Sprintf("%+v", err), "dummy-secret") || strings.Contains(err.Error(), "provider.example") {
		t.Fatalf("unsafe transport error: %v", err)
	}
}

func TestProviderDecodeUsesSharedValidation(t *testing.T) {
	valid := providerPayload(t, providerRow())
	for _, test := range []struct {
		name, body, field string
		syntax, typed     bool
	}{
		{"syntax", `{"data":[?]}`, "", true, false},
		{"wrong price type", strings.Replace(valid, `"lmp":-20`, `"lmp":"dummy-secret"`, 1), "", false, true},
		{"missing lmp", strings.Replace(valid, `"lmp":-20,`, "", 1), "intervals[0].lmp", false, false},
		{"missing data", `{}`, "intervals", false, false},
		{"empty data", `{"data":[]}`, "intervals", false, false},
		{"trailing value", valid + ` {}`, "", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := providerClient(t, &http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) { return providerResponse(test.body), nil })}, "")
			rows, err := client.Fetch(context.Background(), "dummy-secret", providerQuery())
			if err == nil || rows != nil {
				t.Fatalf("rows = %#v, error = %v", rows, err)
			}
			var validation *ValidationError
			var upstream *GridStatusError
			if test.field != "" {
				if !errors.As(err, &validation) || validation.Field != test.field {
					t.Fatalf("validation error = %v", err)
				}
			} else if !errors.As(err, &upstream) || upstream.Code != "UPSTREAM_UNAVAILABLE" {
				t.Fatalf("decode failure = %v", err)
			}
			var syntax *json.SyntaxError
			var typed *json.UnmarshalTypeError
			if test.syntax && !errors.As(err, &syntax) || test.typed && !errors.As(err, &typed) {
				t.Errorf("decode cause lost: %v", err)
			}
			if strings.Contains(err.Error(), "dummy-secret") {
				t.Errorf("decode error leaked credential: %v", err)
			}
		})
	}
}

func TestProviderQueryValidationBeforeIO(t *testing.T) {
	var calls atomic.Int32
	client := providerClient(t, &http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected request")
	})}, "")
	for _, test := range []struct {
		name, field string
		change      func(*Query)
	}{
		{"empty dataset", "dataset_id", func(q *Query) { q.DatasetID = " " }},
		{"empty location", "location_id", func(q *Query) { q.LocationID = "" }},
		{"clock", "timezone", func(q *Query) { q.Clock = "utc" }},
		{"invalid date", "start_date", func(q *Query) { q.StartDate = "2024-02-30" }},
		{"unpadded date", "start_date", func(q *Query) { q.StartDate = "2024-1-01" }},
		{"timestamp", "start_date", func(q *Query) { q.StartDate = "2024-01-01T00:00:00Z" }},
		{"zero year", "start_date", func(q *Query) { q.StartDate = "0000-01-01" }},
		{"bad end", "end_date", func(q *Query) { q.EndDate = "2024-02-30" }},
		{"same day", "end_date", func(q *Query) { q.EndDate = q.StartDate }},
		{"reversed", "end_date", func(q *Query) { q.EndDate = "2023-12-31" }},
		{"61 days", "end_date", func(q *Query) { q.EndDate = "2024-03-02" }},
		{"future", "end_date", func(q *Query) {
			q.StartDate = time.Now().UTC().Format(time.DateOnly)
			q.EndDate = time.Now().UTC().AddDate(0, 0, 1).Format(time.DateOnly)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := providerQuery()
			test.change(&query)
			_, err := client.Fetch(context.Background(), "k", query)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("error = %v", err)
			}
		})
	}
	for _, key := range []string{"", "  \t"} {
		_, err := client.Fetch(context.Background(), key, providerQuery())
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Field != "api_key" {
			t.Errorf("empty credential error = %v", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid requests reached transport %d times", calls.Load())
	}
	query := providerQuery()
	query.EndDate = "2024-03-01"
	if err := validateQuery(query, time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Errorf("60-day leap-year range ending today rejected: %v", err)
	}
}

func TestProviderEnforcesLocationAndSelectedClockRange(t *testing.T) {
	for _, test := range []struct {
		name, clock string
		start       string
		duration    time.Duration
		location    string
		valid       bool
	}{
		{"market start", "market", "2024-01-01T00:00:00-08:00", 5 * time.Minute, "test-node", true},
		{"market exact end", "market", "2024-01-01T23:55:00-08:00", 5 * time.Minute, "test-node", true},
		{"UTC exact end", "UTC", "2024-01-01T15:55:00-08:00", 5 * time.Minute, "test-node", true},
		{"market before start", "market", "2023-12-31T23:55:00-08:00", 5 * time.Minute, "test-node", false},
		{"same instant inside UTC", "UTC", "2023-12-31T23:55:00-08:00", 5 * time.Minute, "test-node", true},
		{"market end before start", "market", "2024-01-01T00:00:00+23:00", 5 * time.Minute, "test-node", false},
		{"market after end", "market", "2024-01-02T00:00:00-08:00", 5 * time.Minute, "test-node", false},
		{"market crossing end", "market", "2024-01-01T23:55:00-08:00", 10 * time.Minute, "test-node", false},
		{"market subsecond after end", "market", "2024-01-01T23:55:00-08:00", 5*time.Minute + time.Nanosecond, "test-node", false},
		{"UTC after end", "UTC", "2024-01-01T23:55:00-08:00", 5 * time.Minute, "test-node", false},
		{"wrong location", "market", "2024-01-01T00:00:00-08:00", 5 * time.Minute, "another-node", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := providerRow()
			start, err := time.Parse(time.RFC3339, test.start)
			if err != nil {
				t.Fatal(err)
			}
			row.IntervalStartLocal, row.IntervalEndLocal = start, start.Add(test.duration)
			row.IntervalStartUTC, row.IntervalEndUTC = start.UTC(), start.Add(test.duration).UTC()
			row.Location = test.location
			if test.name == "market end before start" {
				row.IntervalEndLocal, _ = time.Parse(time.RFC3339, "2023-12-31T23:00:00-23:00")
				row.IntervalEndUTC = row.IntervalEndLocal.UTC()
			}
			body := providerPayload(t, row)
			client := providerClient(t, &http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) { return providerResponse(body), nil })}, "")
			query := providerQuery()
			query.Clock = test.clock
			rows, err := client.Fetch(context.Background(), "k", query)
			if test.valid {
				if err != nil || len(rows) != 1 {
					t.Fatalf("valid boundary rejected: %v", err)
				}
			} else {
				var validation *ValidationError
				if !errors.As(err, &validation) || rows != nil {
					t.Fatalf("invalid boundary accepted: %#v, %v", rows, err)
				}
			}
		})
	}
}

func TestProviderPreservesNormalizationLimitAndOrder(t *testing.T) {
	first := providerRow()
	second := first
	second.IntervalStartUTC, second.IntervalEndUTC = first.IntervalEndUTC, first.IntervalEndUTC.Add(10*time.Minute)
	second.IntervalStartLocal, second.IntervalEndLocal = first.IntervalEndLocal, first.IntervalEndLocal.Add(10*time.Minute)
	body := providerPayload(t, second, first)
	client := providerClient(t, &http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) { return providerResponse(body), nil })}, "")
	rows, err := client.Fetch(context.Background(), "k", providerQuery())
	if err != nil || len(rows) != 2 || !rows[0].IntervalStartUTC.Equal(first.IntervalStartUTC) || rows[1].Duration() != 10*time.Minute {
		t.Fatalf("normalization = %#v, %v", rows, err)
	}
	many := make([]model.LMPInterval, maxSeriesIntervals+1)
	for index := range many {
		many[index] = first
	}
	body = providerPayload(t, many...)
	_, err = client.Fetch(context.Background(), "k", providerQuery())
	if !errors.Is(err, ErrIntervalLimit) {
		t.Fatalf("interval limit identity lost: %v", err)
	}
}

func TestProviderCancellationClosesOutstandingWork(t *testing.T) {
	for _, duringBody := range []bool{false, true} {
		t.Run(fmt.Sprint(duringBody), func(t *testing.T) {
			started, canceled := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if duringBody {
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, `{"data":[`)
					w.(http.Flusher).Flush()
				}
				close(started)
				<-req.Context().Done()
				close(canceled)
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := providerClient(t, server.Client(), server.URL)
			result := make(chan error, 1)
			go func() { _, err := client.Fetch(ctx, "k", providerQuery()); result <- err }()
			waitProviderSignal(t, started)
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel error = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("fetch did not cancel")
			}
			waitProviderSignal(t, canceled)
		})
	}
}

func waitProviderSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("provider barrier timed out")
	}
}

func TestProviderDeadlineAndPreCanceledRequest(t *testing.T) {
	transport := providerTransport(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		if !ok || time.Until(deadline) > providerTimeout || time.Until(deadline) <= 0 {
			t.Error("30-second request cap missing")
		}
		return providerResponse(providerPayload(t, providerRow())), nil
	})
	client := providerClient(t, &http.Client{Transport: transport, Timeout: 2 * time.Minute}, "")
	if _, err := client.Fetch(context.Background(), "k", providerQuery()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	canceledClient := providerClient(t, &http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected request")
	})}, "")
	if _, err := canceledClient.Fetch(ctx, "k", providerQuery()); !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("pre-cancel error = %v, calls = %d", err, calls.Load())
	}

	for _, clientTimeout := range []bool{false, true} {
		t.Run(fmt.Sprint(clientTimeout), func(t *testing.T) {
			canceled := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { <-req.Context().Done(); close(canceled) }))
			defer server.Close()
			injected := server.Client()
			ctx := context.Background()
			if clientTimeout {
				injected.Timeout = 100 * time.Millisecond
			} else {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			_, err := providerClient(t, injected, server.URL).Fetch(ctx, "k", providerQuery())
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("timeout identity lost: %v", err)
			}
			var upstream *GridStatusError
			if !errors.As(err, &upstream) || upstream.Code != "DEADLINE_EXCEEDED" {
				t.Fatalf("timeout classification = %v", err)
			}
			waitProviderSignal(t, canceled)
		})
	}
}

type providerBody struct {
	reader io.Reader
	read   int64
	closed bool
}

func (b *providerBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.read += int64(n)
	return n, err
}
func (b *providerBody) Close() error { b.closed = true; return nil }

type providerSpaces struct{}

func (providerSpaces) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

type providerReadFailure struct{ err error }

func (r providerReadFailure) Read([]byte) (int, error) { return 0, r.err }

func TestProviderBodyBoundAndReadFailure(t *testing.T) {
	for _, declared := range []bool{false, true} {
		t.Run(fmt.Sprint(declared), func(t *testing.T) {
			body := &providerBody{reader: providerSpaces{}}
			transport := providerTransport(func(*http.Request) (*http.Response, error) {
				response := &http.Response{StatusCode: 200, Body: body, Header: make(http.Header), ContentLength: -1}
				if declared {
					response.ContentLength = maxProviderBodyBytes + 1
				}
				return response, nil
			})
			_, err := providerClient(t, &http.Client{Transport: transport}, "").Fetch(context.Background(), "k", providerQuery())
			var upstream *GridStatusError
			if !errors.As(err, &upstream) || upstream.Code != "UPSTREAM_UNAVAILABLE" || !strings.Contains(err.Error(), "64 MiB") {
				t.Fatalf("body limit error = %v", err)
			}
			if !body.closed || body.read > maxProviderBodyBytes+1 || (declared && body.read != 0) {
				t.Fatalf("read = %d, closed = %v", body.read, body.closed)
			}
		})
	}
	cause := errors.New("body dummy-secret")
	body := &providerBody{reader: providerReadFailure{cause}}
	transport := providerTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: body, ContentLength: -1}, nil
	})
	_, err := providerClient(t, &http.Client{Transport: transport}, "").Fetch(context.Background(), "dummy-secret", providerQuery())
	if !errors.Is(err, cause) || strings.Contains(err.Error(), "dummy-secret") || !body.closed {
		t.Fatalf("body error = %v, closed = %v", err, body.closed)
	}
}

func TestProviderNoCacheOrCredentialAliasing(t *testing.T) {
	t.Setenv("ENABLE_GRIDSTATUS_CACHE", "true")
	t.Setenv("GRIDSTATUS_CACHE_TTL", "24h")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		row := providerRow()
		if req.Header.Get("x-api-key") == "second" {
			row.LMP = 200
		} else {
			row.LMP = 100
		}
		_ = json.NewEncoder(w).Encode(model.GridStatusLMPResponse{Data: []model.LMPInterval{row}})
	}))
	defer server.Close()
	client := providerClient(t, server.Client(), server.URL)
	first, err := client.Fetch(context.Background(), "first", providerQuery())
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Fetch(context.Background(), "second", providerQuery())
	if err != nil {
		t.Fatal(err)
	}
	first[0].LMP = 999
	if second[0].LMP != 200 || calls.Load() != 2 {
		t.Fatalf("credentials or responses shared: %#v, calls = %d", second, calls.Load())
	}
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			key, price := "first", 100.0
			if index%2 == 1 {
				key, price = "second", 200
			}
			rows, err := client.Fetch(context.Background(), key, providerQuery())
			if err != nil || len(rows) != 1 || rows[0].LMP != price {
				t.Errorf("concurrent credential %s result = %#v, %v", key, rows, err)
			}
		}(i)
	}
	workers.Wait()
	if calls.Load() != 14 {
		t.Errorf("upstream calls = %d, want 14", calls.Load())
	}
}

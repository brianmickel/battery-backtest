package data

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"battery-backtest/internal/model"
)

const (
	providerTimeout      = 30 * time.Second
	maxProviderBodyBytes = 64 << 20
)

// GridStatusClient owns request policy; credentials belong to individual calls.
type GridStatusClient struct {
	client  http.Client
	baseURL url.URL
}

// NewGridStatusClient copies client without changing its transport, timeout, or
// cookie jar. A nil client uses http.DefaultClient; an empty URL uses Grid Status.
func NewGridStatusClient(client *http.Client, baseURL string) (*GridStatusClient, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = "https://api.gridstatus.io"
	}
	base, err := url.Parse(baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" ||
		base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || base.Opaque != "" {
		return nil, errors.New("provider base URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	owned := *client
	owned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &GridStatusClient{client: owned, baseURL: *base}, nil
}

// Query selects one location over a half-open range of calendar dates.
type Query struct {
	DatasetID, LocationID string
	StartDate, EndDate    string
	Clock                 string
}

// GridStatusError retains upstream status and retry information. Its message is
// safe to log; Unwrap preserves transport, decoding, and context error identity.
type GridStatusError struct {
	StatusCode int
	Code       string
	Message    string
	RetryAfter string
	cause      error
}

func (e *GridStatusError) Error() string { return e.Message }
func (e *GridStatusError) Unwrap() error { return e.cause }

// Fetch authenticates each request independently and returns a normalized series.
func (c *GridStatusClient) Fetch(ctx context.Context, apiKey string, query Query) ([]model.LMPInterval, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, &ValidationError{Field: "api_key", Message: "is required"}
	}
	if err := validateQuery(query, time.Now().UTC()); err != nil {
		return nil, err
	}
	if query.Clock == "" {
		query.Clock = "market"
	}
	ctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()

	u := c.queryURL(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, providerFailure(ctx, "could not create provider request", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, providerFailure(ctx, "provider request failed", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		failure := &GridStatusError{StatusCode: resp.StatusCode, Code: "UPSTREAM_UNAVAILABLE", Message: "provider returned an unsuccessful status"}
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			failure.Code, failure.Message = "UPSTREAM_AUTH", "provider authentication failed"
		case http.StatusTooManyRequests:
			failure.Code, failure.Message = "UPSTREAM_RATE_LIMIT", "provider rate limit exceeded"
			failure.RetryAfter = resp.Header.Get("Retry-After")
		}
		return nil, failure
	}
	if resp.ContentLength > maxProviderBodyBytes {
		return nil, providerFailure(ctx, "provider response exceeds the 64 MiB limit", nil)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderBodyBytes+1))
	if err != nil {
		return nil, providerFailure(ctx, "could not read provider response", err)
	}
	if len(body) > maxProviderBodyBytes {
		return nil, providerFailure(ctx, "provider response exceeds the 64 MiB limit", nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := DecodeGridStatusJSON(bytes.NewReader(body))
	if err != nil {
		var validation *ValidationError
		if errors.As(err, &validation) || errors.Is(err, ErrIntervalLimit) {
			return nil, err
		}
		return nil, providerFailure(ctx, "provider response is not valid market JSON", err)
	}
	rows, err = NormalizeSeries(rows, query.Clock)
	if err != nil {
		return nil, err
	}
	for index, row := range rows {
		if row.Location != query.LocationID {
			return nil, withIntervalIndex(index, &ValidationError{Field: "location", Message: "does not match the requested location"})
		}
		start, end := row.IntervalStartLocal, row.IntervalEndLocal
		if query.Clock == "UTC" {
			start, end = row.IntervalStartUTC, row.IntervalEndUTC
		}
		startDate, endDate := start.Format(time.DateOnly), end.Format(time.DateOnly)
		if startDate < query.StartDate || startDate >= query.EndDate || endDate < query.StartDate || endDate > query.EndDate ||
			(endDate == query.EndDate && (end.Hour() != 0 || end.Minute() != 0 || end.Second() != 0 || end.Nanosecond() != 0)) {
			return nil, withIntervalIndex(index, &ValidationError{Message: "is outside the requested date range"})
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *GridStatusClient) queryURL(query Query) url.URL {
	u := c.baseURL
	path := strings.TrimRight(u.EscapedPath(), "/") + "/v1/datasets/" + escapeSegment(query.DatasetID) + "/query/location/" + escapeSegment(query.LocationID)
	u.Path, _ = url.PathUnescape(path)
	u.RawPath = path
	u.RawQuery = url.Values{
		"start_time": {query.StartDate},
		"end_time":   {query.EndDate},
		"timezone":   {query.Clock},
		"download":   {"true"},
	}.Encode()
	return u
}

func escapeSegment(value string) string {
	if value == "." || value == ".." {
		return strings.ReplaceAll(value, ".", "%2E")
	}
	return url.PathEscape(value)
}

func validateQuery(query Query, now time.Time) error {
	for _, id := range []struct{ field, value string }{{"dataset_id", query.DatasetID}, {"location_id", query.LocationID}} {
		if strings.TrimSpace(id.value) == "" {
			return &ValidationError{Field: id.field, Message: "is required"}
		}
	}
	if query.Clock != "" && query.Clock != "market" && query.Clock != "UTC" {
		return &ValidationError{Field: "timezone", Message: "must be market or UTC"}
	}
	start, err := time.Parse(time.DateOnly, query.StartDate)
	if err != nil || start.Year() < 1 || start.Format(time.DateOnly) != query.StartDate {
		return &ValidationError{Field: "start_date", Message: "must be a Gregorian date in YYYY-MM-DD format"}
	}
	end, err := time.Parse(time.DateOnly, query.EndDate)
	if err != nil || end.Year() < 1 || end.Format(time.DateOnly) != query.EndDate {
		return &ValidationError{Field: "end_date", Message: "must be a Gregorian date in YYYY-MM-DD format"}
	}
	if !start.Before(end) {
		return &ValidationError{Field: "end_date", Message: "must be after start_date (exclusive end)"}
	}
	if end.After(start.AddDate(0, 0, 60)) {
		return &ValidationError{Field: "end_date", Message: "range must not exceed 60 calendar days"}
	}
	if query.EndDate > now.UTC().Format(time.DateOnly) {
		return &ValidationError{Field: "end_date", Message: "must not be after today's UTC date"}
	}
	return nil
}

func providerFailure(ctx context.Context, message string, cause error) error {
	if err := ctx.Err(); err != nil && !errors.Is(cause, err) {
		cause = errors.Join(cause, err)
	}
	code := "UPSTREAM_UNAVAILABLE"
	if errors.Is(cause, context.DeadlineExceeded) {
		code = "DEADLINE_EXCEEDED"
	}
	return &GridStatusError{Code: code, Message: message, cause: cause}
}

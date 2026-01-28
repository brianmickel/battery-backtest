package data

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"battery-backtest/internal/model"
)

// GridStatusClient provides methods to fetch data from the Grid Status API.
type GridStatusClient struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

// NewGridStatusClient creates a new Grid Status API client.
// If baseURL is empty, defaults to "https://api.gridstatus.io".
func NewGridStatusClient(apiKey string, baseURL string) *GridStatusClient {
	if baseURL == "" {
		baseURL = "https://api.gridstatus.io"
	}
	return &GridStatusClient{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// QueryLocationParams defines parameters for querying location data.
type QueryLocationParams struct {
	DatasetID  string    // e.g., "caiso_lmp_real_time_5_min"
	LocationID string    // e.g., "MOSSLD_2_PSP1" or "SV_LNODER6A"
	StartTime  time.Time // Start of time range
	EndTime    time.Time // End of time range
	Timezone   string    // e.g., "market", "UTC" (default: "market")
	Download   bool      // If true, sets download=true query param
}

// GridStatusError represents an error from the Grid Status API
type GridStatusError struct {
	StatusCode int
	Code       string
	Message    string
	RetryAfter string // For rate limit errors
}

func (e *GridStatusError) Error() string {
	return e.Message
}

// QueryLocation fetches LMP data for a specific location from Grid Status API.
// 
// WARNING: If caching is enabled (ENABLE_GRIDSTATUS_CACHE=true), responses may be cached.
// Caching is ONLY for LOCAL DEVELOPMENT. Check Grid Status Terms of Use before enabling
// in any production-like environment. Caching API responses may violate their terms.
func (c *GridStatusClient) QueryLocation(params QueryLocationParams) (*model.GridStatusLMPResponse, error) {
	// Validate API key before making request
	if err := c.validateAPIKey(); err != nil {
		return nil, err
	}

	// Check cache first (only if enabled for development)
	cache := GetCache()
	if cache != nil {
		cacheKey := GenerateCacheKey(params)
		if cached, found := cache.Get(cacheKey); found {
			// Return cached response
			dataCount := 0
			if cached.Data != nil {
				dataCount = len(cached.Data)
			}
			log.Printf("[GridStatus] Cache hit: Using cached response with %d intervals (dataset=%s, location=%s, start=%s, end=%s)",
				dataCount, params.DatasetID, params.LocationID,
				params.StartTime.Format("2006-01-02"), params.EndTime.Format("2006-01-02"))
			return cached, nil
		}
	}
	if params.DatasetID == "" {
		return nil, fmt.Errorf("dataset_id is required")
	}
	if params.LocationID == "" {
		return nil, fmt.Errorf("location_id is required")
	}
	if params.StartTime.IsZero() || params.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time and end_time are required")
	}
	if params.StartTime.After(params.EndTime) {
		return nil, fmt.Errorf("start_time must be before end_time")
	}

	// Build URL: /v1/datasets/{dataset_id}/query/location/{location_id}
	path := fmt.Sprintf("/v1/datasets/%s/query/location/%s", params.DatasetID, params.LocationID)
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Build query parameters
	q := u.Query()
	q.Set("start_time", params.StartTime.Format("2006-01-02"))
	q.Set("end_time", params.EndTime.Format("2006-01-02"))
	if params.Timezone != "" {
		q.Set("timezone", params.Timezone)
	} else {
		q.Set("timezone", "market")
	}
	if params.Download {
		q.Set("download", "true")
	}
	u.RawQuery = q.Encode()

	// Log the request
	log.Printf("[GridStatus] Request: GET %s (dataset=%s, location=%s, start=%s, end=%s, timezone=%s)",
		u.Path,
		params.DatasetID,
		params.LocationID,
		params.StartTime.Format("2006-01-02"),
		params.EndTime.Format("2006-01-02"),
		q.Get("timezone"))

	// Create HTTP request
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set API key header
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "application/json")

	// Execute request
	startTime := time.Now()
	resp, err := c.Client.Do(req)
	duration := time.Since(startTime)
	if err != nil {
		log.Printf("[GridStatus] Request failed: %v (duration: %v)", err, duration)
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Log the response
	log.Printf("[GridStatus] Response: %d %s (duration: %v, dataset=%s, location=%s)",
		resp.StatusCode,
		resp.Status,
		duration,
		params.DatasetID,
		params.LocationID)

	// Check status code and handle specific errors
	switch resp.StatusCode {
	case http.StatusOK:
		// Success, continue
	case http.StatusForbidden:
		// 403: Invalid API key or insufficient permissions
		log.Printf("[GridStatus] Error: 403 Forbidden - Invalid API key or insufficient permissions (dataset=%s, location=%s)",
			params.DatasetID, params.LocationID)
		return nil, &GridStatusError{
			StatusCode: resp.StatusCode,
			Code:       "INVALID_API_KEY",
			Message:    "Invalid API key or insufficient permissions",
		}
	case http.StatusTooManyRequests:
		// 429: Rate limit exceeded
		retryAfter := resp.Header.Get("Retry-After")
		log.Printf("[GridStatus] Error: 429 Rate Limit Exceeded - Retry after: %s (dataset=%s, location=%s)",
			retryAfter, params.DatasetID, params.LocationID)
		return nil, &GridStatusError{
			StatusCode: resp.StatusCode,
			Code:       "RATE_LIMIT_EXCEEDED",
			Message:    fmt.Sprintf("Rate limit exceeded. Retry after: %s", retryAfter),
			RetryAfter: retryAfter,
		}
	case http.StatusUnauthorized:
		// 401: Unauthorized (bad API key)
		log.Printf("[GridStatus] Error: 401 Unauthorized - Invalid API key (dataset=%s, location=%s)",
			params.DatasetID, params.LocationID)
		return nil, &GridStatusError{
			StatusCode: resp.StatusCode,
			Code:       "UNAUTHORIZED",
			Message:    "Unauthorized: Invalid API key",
		}
	default:
		// Other errors
		log.Printf("[GridStatus] Error: %d %s (dataset=%s, location=%s)",
			resp.StatusCode, resp.Status, params.DatasetID, params.LocationID)
		return nil, &GridStatusError{
			StatusCode: resp.StatusCode,
			Code:       "API_ERROR",
			Message:    fmt.Sprintf("API returned status %d: %s", resp.StatusCode, resp.Status),
		}
	}

	// Parse JSON response
	var result model.GridStatusLMPResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[GridStatus] Error decoding response: %v (dataset=%s, location=%s)", err, params.DatasetID, params.LocationID)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Log successful response with data count
	dataCount := 0
	if result.Data != nil {
		dataCount = len(result.Data)
	}
	log.Printf("[GridStatus] Success: Received %d intervals (dataset=%s, location=%s)",
		dataCount, params.DatasetID, params.LocationID)

	// Cache the response if caching is enabled (development only)
	if cache := GetCache(); cache != nil {
		cacheKey := GenerateCacheKey(params)
		cache.Set(cacheKey, &result)
		log.Printf("[GridStatus] Cached response (dataset=%s, location=%s)", params.DatasetID, params.LocationID)
	}

	return &result, nil
}

// validateAPIKey validates that the API key is present and not obviously invalid
func (c *GridStatusClient) validateAPIKey() error {
	if c.APIKey == "" {
		return &GridStatusError{
			StatusCode: 0,
			Code:       "MISSING_API_KEY",
			Message:    "API key is required",
		}
	}
	// Basic validation: API key should not be just whitespace
	// Grid Status API keys are typically non-empty strings
	// We don't validate format here, but reject obviously bad keys
	if len(c.APIKey) < 10 {
		return &GridStatusError{
			StatusCode: 0,
			Code:       "INVALID_API_KEY_FORMAT",
			Message:    "API key appears to be invalid (too short)",
		}
	}
	return nil
}

// QueryLocationByString is a convenience method that parses date strings.
// startDate and endDate should be in "YYYY-MM-DD" format.
func (c *GridStatusClient) QueryLocationByString(datasetID, locationID, startDate, endDate string) (*model.GridStatusLMPResponse, error) {
	startTime, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start_date format (expected YYYY-MM-DD): %w", err)
	}
	endTime, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end_date format (expected YYYY-MM-DD): %w", err)
	}

	return c.QueryLocation(QueryLocationParams{
		DatasetID:  datasetID,
		LocationID: locationID,
		StartTime:  startTime,
		EndTime:    endTime,
		Timezone:   "market",
		Download:   true,
	})
}

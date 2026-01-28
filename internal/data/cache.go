package data

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	"battery-backtest/internal/model"
)

// CacheEntry represents a cached API response
type CacheEntry struct {
	Response *model.GridStatusLMPResponse
	ExpiresAt time.Time
}

// ResponseCache provides in-memory caching for Grid Status API responses.
//
// ⚠️ WARNING: This cache is for LOCAL DEVELOPMENT ONLY.
//
// Caching API responses may violate Grid Status Terms of Use.
// Before enabling this feature:
//   1. Review Grid Status Terms of Use
//   2. Confirm caching is allowed for your use case
//   3. Only enable in local development environments
//   4. Never enable in production without explicit permission
//
// This cache is automatically disabled when API_ENV=production.
type ResponseCache struct {
	mu    sync.RWMutex
	store map[string]*CacheEntry
	ttl   time.Duration
}

var globalCache *ResponseCache
var cacheOnce sync.Once

// GetCache returns the global cache instance if caching is enabled.
// Returns nil if caching is disabled.
//
// ⚠️ DEVELOPMENT ONLY: This cache is automatically disabled in production.
// Check Grid Status Terms of Use before enabling.
func GetCache() *ResponseCache {
	// Only enable cache if explicitly enabled via environment variable
	// AND only in development mode
	if os.Getenv("ENABLE_GRIDSTATUS_CACHE") != "true" {
		return nil
	}
	
	// Additional safety check: only enable in development
	// This prevents accidental enabling in production
	env := os.Getenv("API_ENV")
	if env == "production" {
		return nil
	}

	cacheOnce.Do(func() {
		ttl := 1 * time.Hour // Default TTL: 1 hour
		if ttlStr := os.Getenv("GRIDSTATUS_CACHE_TTL"); ttlStr != "" {
			if parsed, err := time.ParseDuration(ttlStr); err == nil {
				ttl = parsed
			}
		}
		
		globalCache = &ResponseCache{
			store: make(map[string]*CacheEntry),
			ttl:   ttl,
		}
		
		// Start cleanup goroutine
		go globalCache.cleanup()
	})
	
	return globalCache
}

// Get retrieves a cached response if available and not expired
func (c *ResponseCache) Get(key string) (*model.GridStatusLMPResponse, bool) {
	if c == nil {
		return nil, false
	}
	
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.store[key]
	if !exists {
		return nil, false
	}
	
	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	
	return entry.Response, true
}

// Set stores a response in the cache
func (c *ResponseCache) Set(key string, response *model.GridStatusLMPResponse) {
	if c == nil {
		return
	}
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.store[key] = &CacheEntry{
		Response:  response,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// Clear removes all entries from the cache
func (c *ResponseCache) Clear() {
	if c == nil {
		return
	}
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.store = make(map[string]*CacheEntry)
}

// cleanup periodically removes expired entries
func (c *ResponseCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.store {
			if now.After(entry.ExpiresAt) {
				delete(c.store, key)
			}
		}
		c.mu.Unlock()
	}
}

// GenerateCacheKey creates a cache key from query parameters
func GenerateCacheKey(params QueryLocationParams) string {
	// Create a deterministic key from all parameters
	keyStr := fmt.Sprintf("%s:%s:%s:%s:%s:%v",
		params.DatasetID,
		params.LocationID,
		params.StartTime.Format("2006-01-02"),
		params.EndTime.Format("2006-01-02"),
		params.Timezone,
		params.Download,
	)
	
	// Hash the key to keep it reasonably sized
	hash := sha256.Sum256([]byte(keyStr))
	return hex.EncodeToString(hash[:])
}

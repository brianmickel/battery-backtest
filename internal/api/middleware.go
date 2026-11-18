package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

var expensivePermit = make(chan struct{}, 1)

func admit(c *gin.Context) (context.Context, func(), bool) {
	if err := c.Request.Context().Err(); err != nil {
		writeFailure(c, err)
		return nil, nil, false
	}
	select {
	case expensivePermit <- struct{}{}:
		return c.Request.Context(), func() { <-expensivePermit }, true
	default:
		writeFailure(c, failure{status: 503, code: "SERVER_BUSY", message: "Another experiment is running. Try again shortly.", retry: "1"})
		return nil, nil, false
	}
}

func accessAndRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		defer func() {
			if recover() != nil {
				writeFailure(c, failure{status: 500, code: "INTERNAL_ERROR", message: "The operation could not be completed."})
			}
			route := c.FullPath()
			if route == "" {
				route = "unmatched"
			}
			log.Printf("method=%s path=%s status=%d error_code=%s elapsed=%s", c.Request.Method, route, c.Writer.Status(), c.GetString("error_code"), time.Since(start))
		}()
		c.Next()
	}
}

func cors(origins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Writer.Header().Add("Vary", "Origin")
		}
		if allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-GridStatus-API-Key")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

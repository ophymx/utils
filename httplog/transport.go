package httplog

import (
	"log"
	"net/http"
	"time"
)

// LoggingTransport wraps an http.RoundTripper to log outbound HTTP requests and responses
type LoggingTransport struct {
	Transport http.RoundTripper
}

// RoundTrip executes the HTTP request and logs the details
func (lt *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	// Log the outbound request
	log.Printf("→ %s %s %s", req.Method, req.URL.String(), req.Proto)
	if req.Header.Get("User-Agent") != "" {
		log.Printf("→ User-Agent: %s", req.Header.Get("User-Agent"))
	}
	if req.Header.Get("Referer") != "" {
		log.Printf("→ Referer: %s", req.Header.Get("Referer"))
	}
	if req.ContentLength > 0 {
		log.Printf("→ Content-Length: %d", req.ContentLength)
	}

	// Execute the request
	resp, err := lt.Transport.RoundTrip(req)

	duration := time.Since(start)

	if err != nil {
		log.Printf("← ERROR: %s %s - %v (took %v)", req.Method, req.URL.String(), err, duration)
		return nil, err
	}

	// Log the response
	log.Printf("← %s %s - %d %s (took %v)", req.Method, req.URL.String(), resp.StatusCode, resp.Status, duration)
	if resp.Header.Get("Content-Type") != "" {
		log.Printf("← Content-Type: %s", resp.Header.Get("Content-Type"))
	}
	if resp.ContentLength > 0 {
		log.Printf("← Content-Length: %d", resp.ContentLength)
	}

	return resp, nil
}

// NewLoggingTransport creates a new LoggingTransport with the default transport
func NewLoggingTransport() *LoggingTransport {
	return &LoggingTransport{
		Transport: http.DefaultTransport,
	}
}

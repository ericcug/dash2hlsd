package dash

import (
	"dash2hlsd/internal/logger"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the DASH client responsible for all communication with the origin server.
type Client struct {
	httpClient *http.Client
	logger     logger.Logger
}

// NewClient creates a new DASH client.
// It configures an http.Client with specific timeouts as per the design document
// to handle potentially poor network conditions. This client should be reused across the application.
func NewClient(log logger.Logger) *Client {
	// As per the design, the transport is configured for fast-fail scenarios.
	transport := &http.Transport{
		ResponseHeaderTimeout: 2 * time.Second, // Timeout for receiving response headers.
		// Other transport settings like MaxIdleConns, IdleConnTimeout can be tuned here.
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			// The 5-second total download timeout will be handled per-request using context.
		},
		logger: log,
	}
}

// FetchAndParseMPD fetches the MPD from a given URL and parses it into the MPD struct.
func (c *Client) FetchAndParseMPD(url, userAgent string) (*MPD, error) {
	c.logger.Debugf("Fetching MPD from URL: %s", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create new request for MPD: %w", err)
	}

	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch MPD from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch MPD: received status code %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read MPD response body: %w", err)
	}

	var mpd MPD
	if err := xml.Unmarshal(data, &mpd); err != nil {
		return nil, fmt.Errorf("failed to unmarshal MPD XML: %w", err)
	}

	c.logger.Debugf("Successfully fetched and parsed MPD for profile %s", mpd.Profiles)
	return &mpd, nil
}

// HttpClient returns the underlying http.Client instance.
func (c *Client) HttpClient() *http.Client {
	return c.httpClient
}

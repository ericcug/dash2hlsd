package dash

import (
	"dash2hlsd/internal/logger"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the DASH client responsible for all communication with the origin server.
type Client struct {
	httpClient *http.Client
	logger     logger.Logger
}

// NewClient creates a new DASH client.
func NewClient(log logger.Logger) *Client {
	transport := &http.Transport{
		ResponseHeaderTimeout: 3 * time.Second,
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		logger: log,
	}
}

// FetchAndParseMPD fetches the MPD from a given URL and parses it into the MPD struct.
func (c *Client) FetchAndParseMPD(initialUrl, userAgent string) (*MPD, string, error) {
	c.logger.Debugf("Fetching MPD from URL: %s", initialUrl)

	req, err := http.NewRequest("GET", initialUrl, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create new request for MPD: %w", err)
	}

	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch MPD from %s: %w", initialUrl, err)
	}
	defer resp.Body.Close()

	finalUrl := initialUrl
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		location, err := resp.Location()
		if err != nil {
			return nil, "", fmt.Errorf("redirect location error: %w", err)
		}
		finalUrl = location.String()
		c.logger.Debugf("Redirected to: %s", finalUrl)

		req, err = http.NewRequest("GET", finalUrl, nil)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create new request for redirected MPD: %w", err)
		}
		if userAgent != "" {
			req.Header.Set("User-Agent", userAgent)
		}

		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("failed to fetch redirected MPD from %s: %w", finalUrl, err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to fetch MPD: received status code %d from %s", resp.StatusCode, finalUrl)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read MPD response body: %w", err)
	}

	var mpd MPD
	if err := xml.Unmarshal(data, &mpd); err != nil {
		c.logger.Errorf("Failed to unmarshal MPD XML from %s: %v. XML data: %s", finalUrl, err, string(data))
		return nil, "", fmt.Errorf("failed to unmarshal MPD XML: %w", err)
	}

	c.logger.Debugf("Successfully fetched and parsed MPD for profile %s from %s", mpd.Profiles, finalUrl)
	return &mpd, finalUrl, nil
}

// HttpClient returns the underlying http.Client instance.
func (c *Client) HttpClient() *http.Client {
	return c.httpClient
}

// resolveURL resolves a path against a base URL, handling potential errors.
func resolveURL(base *url.URL, path string) (*url.URL, error) {
	resolvedPath, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse path '%s': %w", path, err)
	}
	return base.ResolveReference(resolvedPath), nil
}

// BuildInitSegmentURL constructs the full URL for an initialization segment.
// It correctly resolves against the MPD location and the Period's BaseURL tag.
func BuildInitSegmentURL(mpdLocationURL string, period *Period, as *AdaptationSet, rep *Representation) (string, error) {
	mpdURL, err := url.Parse(mpdLocationURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse mpdLocationURL '%s': %w", mpdLocationURL, err)
	}

	currentBase := mpdURL
	if period.BaseURL != "" {
		currentBase, err = resolveURL(mpdURL, period.BaseURL)
		if err != nil {
			return "", fmt.Errorf("failed to resolve period BaseURL: %w", err)
		}
	}

	initPath := strings.Replace(as.SegmentTemplate.Initialization, "$RepresentationID$", rep.ID, 1)
	finalURL, err := resolveURL(currentBase, initPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve init path: %w", err)
	}

	return finalURL.String(), nil
}

// BuildSegmentURL constructs the full URL for a media segment.
// It correctly resolves against the MPD location and the Period's BaseURL tag.
func BuildSegmentURL(mpdLocationURL string, period *Period, as *AdaptationSet, rep *Representation, time uint64) (string, error) {
	mpdURL, err := url.Parse(mpdLocationURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse mpdLocationURL '%s': %w", mpdLocationURL, err)
	}

	currentBase := mpdURL
	if period.BaseURL != "" {
		currentBase, err = resolveURL(mpdURL, period.BaseURL)
		if err != nil {
			return "", fmt.Errorf("failed to resolve period BaseURL: %w", err)
		}
	}

	mediaPath := strings.Replace(as.SegmentTemplate.Media, "$RepresentationID$", rep.ID, 1)
	mediaPath = strings.Replace(mediaPath, "$Time$", fmt.Sprintf("%d", time), 1)
	finalURL, err := resolveURL(currentBase, mediaPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve media path: %w", err)
	}

	return finalURL.String(), nil
}

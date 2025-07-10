package dash

import (
	"context"
	"dash2hlsd/internal/logger"
	"dash2hlsd/internal/models"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SegmentDownloader is responsible for downloading individual media segments with robust retry logic.
type SegmentDownloader struct {
	httpClient *http.Client
	logger     logger.Logger
	userAgent  string
}

// NewSegmentDownloader creates a new downloader.
func NewSegmentDownloader(client *http.Client, log logger.Logger, userAgent string) *SegmentDownloader {
	return &SegmentDownloader{
		httpClient: client,
		logger:     log,
		userAgent:  userAgent,
	}
}

// DownloadSegment fetches a single media segment with context-based timeout and retries.
func (sd *SegmentDownloader) DownloadSegment(segment models.Segment) ([]byte, error) {
	const maxRetries = 3
	const retryDelay = 100 * time.Millisecond
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Per-request timeout as specified in the design document.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", segment.URL, nil)
		if err != nil {
			// This error is non-recoverable, so don't retry.
			return nil, fmt.Errorf("failed to create request for segment %s: %w", segment.ID, err)
		}

		if sd.userAgent != "" {
			req.Header.Set("User-Agent", sd.userAgent)
		}

		sd.logger.Debugf("Downloading segment %s (Attempt %d/%d)", segment.ID, attempt, maxRetries)
		resp, err := sd.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("download attempt %d failed for segment %s: %w", attempt, segment.ID, err)
			sd.logger.Warnf(lastErr.Error())
			time.Sleep(retryDelay) // Wait before retrying
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("download attempt %d for segment %s received non-200 status: %d", attempt, segment.ID, resp.StatusCode)
			sd.logger.Warnf(lastErr.Error())
			time.Sleep(retryDelay)
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("download attempt %d for segment %s failed while reading body: %w", attempt, segment.ID, err)
			sd.logger.Warnf(lastErr.Error())
			time.Sleep(retryDelay)
			continue
		}

		sd.logger.Debugf("Successfully downloaded segment %s", segment.ID)
		return data, nil
	}

	return nil, fmt.Errorf("failed to download segment %s after %d attempts: %w", segment.ID, maxRetries, lastErr)
}

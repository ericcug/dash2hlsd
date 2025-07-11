package main_test

import (
	"dash2hlsd/internal/dash"
	"dash2hlsd/internal/models"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockLogger is a no-op logger for testing purposes.
type downloaderMockLogger struct{}

func (m *downloaderMockLogger) Debugf(format string, v ...interface{}) {}
func (m *downloaderMockLogger) Infof(format string, v ...interface{})  {}
func (m *downloaderMockLogger) Warnf(format string, v ...interface{})  {}
func (m *downloaderMockLogger) Errorf(format string, v ...interface{}) {}

// TestDownloader_Success verifies a successful download on the first attempt.
func TestDownloader_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "segment data")
	}))
	defer server.Close()

	client := dash.NewClient(&downloaderMockLogger{})
	downloader := dash.NewDownloader(client.HttpClient(), &downloaderMockLogger{}, "test-agent", 2)
	defer downloader.Stop()

	results := make(chan dash.DownloadResult, 1)
	segment := models.Segment{URL: server.URL, ID: "1"}

	downloader.QueueDownload(dash.DownloadTask{Segment: segment, Result: results})

	result := <-results
	assert.NoError(t, result.Error)
	assert.Equal(t, "segment data", string(result.Data))
}

// TestDownloader_RetryThenSuccess verifies that the downloader retries on failure and succeeds.
func TestDownloader_RetryThenSuccess(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, "final segment data")
	}))
	defer server.Close()

	client := dash.NewClient(&downloaderMockLogger{})
	downloader := dash.NewDownloader(client.HttpClient(), &downloaderMockLogger{}, "test-agent", 1)
	defer downloader.Stop()

	results := make(chan dash.DownloadResult, 1)
	segment := models.Segment{URL: server.URL, ID: "2"}

	downloader.QueueDownload(dash.DownloadTask{Segment: segment, Result: results})

	result := <-results
	assert.NoError(t, result.Error)
	assert.Equal(t, "final segment data", string(result.Data))
	assert.Equal(t, int32(3), requestCount, "Expected exactly 3 attempts")
}

// TestDownloader_Timeout verifies that the per-request timeout is respected.
func TestDownloader_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Exceeds the timeout
		fmt.Fprint(w, "this should not be sent")
	}))
	defer server.Close()

	client := dash.NewClient(&downloaderMockLogger{})
	downloader := dash.NewDownloader(client.HttpClient(), &downloaderMockLogger{}, "test-agent", 1)
	downloader.RequestTimeout = 100 * time.Millisecond // Set a short timeout for the test
	defer downloader.Stop()

	results := make(chan dash.DownloadResult, 1)
	segment := models.Segment{URL: server.URL, ID: "3"}

	downloader.QueueDownload(dash.DownloadTask{Segment: segment, Result: results})

	// We need to wait for the result.
	select {
	case result := <-results:
		assert.Error(t, result.Error)
		assert.Contains(t, result.Error.Error(), "context deadline exceeded")
	case <-time.After(2 * time.Second): // Test timeout
		t.Fatal("Test timed out waiting for download result")
	}
}

// TestDownloader_FailureAfterRetries verifies that the downloader fails after all retries.
func TestDownloader_FailureAfterRetries(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := dash.NewClient(&downloaderMockLogger{})
	downloader := dash.NewDownloader(client.HttpClient(), &downloaderMockLogger{}, "test-agent", 1)
	defer downloader.Stop()

	results := make(chan dash.DownloadResult, 1)
	segment := models.Segment{URL: server.URL, ID: "4"}

	downloader.QueueDownload(dash.DownloadTask{Segment: segment, Result: results})

	result := <-results
	assert.Error(t, result.Error)
	assert.Equal(t, int32(3), atomic.LoadInt32(&requestCount), "Expected exactly 3 attempts")
	assert.Contains(t, result.Error.Error(), "failed to download segment 4 after 3 attempts")
}

package dash

import (
	"context"
	"dash2hlsd/internal/logger"
	"dash2hlsd/internal/models"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// DownloadTask represents a segment to be downloaded.
type DownloadTask struct {
	Segment models.Segment
	Result  chan<- DownloadResult
}

// DownloadResult holds the result of a download attempt.
type DownloadResult struct {
	Task  DownloadTask
	Data  []byte
	Error error
}

// Downloader is responsible for managing concurrent segment downloads.
type Downloader struct {
	httpClient     *http.Client
	logger         logger.Logger
	userAgent      string
	taskQueue      chan DownloadTask
	workerWG       sync.WaitGroup
	maxRetries     int
	retryDelay     time.Duration
	RequestTimeout time.Duration
}

// NewDownloader creates a new downloader with a worker pool.
func NewDownloader(client *http.Client, log logger.Logger, userAgent string, numWorkers int) *Downloader {
	d := &Downloader{
		httpClient:     client,
		logger:         log,
		userAgent:      userAgent,
		taskQueue:      make(chan DownloadTask, 100), // Buffered channel
		maxRetries:     3,
		retryDelay:     200 * time.Millisecond,
		RequestTimeout: 10 * time.Second,
	}

	d.workerWG.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go d.worker(i + 1)
	}

	return d
}

// QueueDownload adds a segment to the download queue.
func (d *Downloader) QueueDownload(task DownloadTask) {
	d.taskQueue <- task
}

// Stop gracefully shuts down the downloader and its workers.
func (d *Downloader) Stop() {
	close(d.taskQueue)
	d.workerWG.Wait()
}

func (d *Downloader) worker(id int) {
	defer d.workerWG.Done()
	d.logger.Debugf("Worker %d started", id)

	for task := range d.taskQueue {
		data, err := d.download(task.Segment)
		task.Result <- DownloadResult{
			Task:  task,
			Data:  data,
			Error: err,
		}
	}

	d.logger.Debugf("Worker %d finished", id)
}

func (d *Downloader) download(segment models.Segment) ([]byte, error) {
	var lastErr error

	for attempt := 1; attempt <= d.maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), d.RequestTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", segment.URL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request for segment %s: %w", segment.ID, err)
		}

		if d.userAgent != "" {
			req.Header.Set("User-Agent", d.userAgent)
		}

		d.logger.Debugf("Downloading segment %s from %s (Attempt %d/%d)", segment.ID, segment.URL, attempt, d.maxRetries)
		resp, err := d.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("download attempt %d for segment %s (%s) failed: %w", attempt, segment.ID, segment.URL, err)
			d.logger.Warnf(lastErr.Error())
			time.Sleep(d.retryDelay)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("download attempt %d for segment %s (%s) received non-200 status: %d", attempt, segment.ID, segment.URL, resp.StatusCode)
			d.logger.Warnf(lastErr.Error())
			time.Sleep(d.retryDelay)
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("download attempt %d for segment %s (%s) failed while reading body: %w", attempt, segment.ID, segment.URL, err)
			d.logger.Warnf(lastErr.Error())
			time.Sleep(d.retryDelay)
			continue
		}

		d.logger.Debugf("Successfully downloaded segment %s", segment.ID)
		return data, nil
	}

	return nil, fmt.Errorf("failed to download segment %s after %d attempts: %w", segment.ID, d.maxRetries, lastErr)
}

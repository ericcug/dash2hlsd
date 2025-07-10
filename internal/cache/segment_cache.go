package cache

import (
	"context"
	"dash2hlsd/internal/logger"
	"sync"
	"time"
)

// ActiveSegmentsProvider is a function type that provides a set of all currently active segment keys.
type ActiveSegmentsProvider func() map[string]struct{}

// SegmentCache provides a thread-safe, in-memory cache for media segments.
type SegmentCache struct {
	mutex                  sync.RWMutex
	cache                  map[string][]byte
	logger                 logger.Logger
	activeSegmentsProvider ActiveSegmentsProvider

	// Control
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates and returns a new SegmentCache.
func New(log logger.Logger, provider ActiveSegmentsProvider) *SegmentCache {
	ctx, cancel := context.WithCancel(context.Background())
	return &SegmentCache{
		cache:                  make(map[string][]byte),
		logger:                 log,
		activeSegmentsProvider: provider,
		ctx:                    ctx,
		cancel:                 cancel,
	}
}

// Start begins the background eviction worker.
func (sc *SegmentCache) Start() {
	sc.logger.Infof("Starting segment cache eviction worker...")
	go sc.evictionWorker()
}

// Stop gracefully shuts down the eviction worker.
func (sc *SegmentCache) Stop() {
	sc.logger.Infof("Stopping segment cache eviction worker...")
	sc.cancel()
}

// Set adds a segment to the cache.
func (sc *SegmentCache) Set(key string, data []byte) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	sc.cache[key] = data
	sc.logger.Debugf("Cached segment: %s, size: %d bytes", key, len(data))
}

// Get retrieves a segment from the cache.
func (sc *SegmentCache) Get(key string) ([]byte, bool) {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	data, found := sc.cache[key]
	return data, found
}

// evictionWorker runs in the background to clean up expired segments.
func (sc *SegmentCache) evictionWorker() {
	ticker := time.NewTicker(10 * time.Second) // Run eviction every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-sc.ctx.Done():
			sc.logger.Infof("Eviction worker stopped.")
			return
		case <-ticker.C:
			sc.runEviction()
		}
	}
}

func (sc *SegmentCache) runEviction() {
	sc.logger.Debugf("Running cache eviction...")
	activeKeys := sc.activeSegmentsProvider()

	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	evictedCount := 0
	for key := range sc.cache {
		if _, isActive := activeKeys[key]; !isActive {
			delete(sc.cache, key)
			evictedCount++
		}
	}

	if evictedCount > 0 {
		sc.logger.Infof("Evicted %d segments from cache. Current cache size: %d segments.", evictedCount, len(sc.cache))
	} else {
		sc.logger.Debugf("No segments to evict. Current cache size: %d segments.", len(sc.cache))
	}
}

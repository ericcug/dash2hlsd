package session

import (
	"context"
	"dash2hlsd/internal/cache"
	"dash2hlsd/internal/channels"
	"dash2hlsd/internal/dash"
	"dash2hlsd/internal/hls"
	"dash2hlsd/internal/logger"
	"dash2hlsd/internal/models"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	playlistLiveSegments = 5 // Number of segments to include in the live playlist
)

// StreamSession holds all context for a single live stream.
type StreamSession struct {
	ChannelID   string
	ManifestURL string
	BaseURL     string // The final URL after any redirects
	Logger      logger.Logger
	MPD         *dash.MPD
	Downloader  *dash.Downloader
	SegCache    *cache.SegmentCache

	// Thread-safe state
	mutex             sync.RWMutex
	availableSegments map[string][]*models.Segment // Keyed by Representation ID
	playlistCache     map[string]string            // Keyed by Representation ID
	mediaSequence     map[string]int               // Keyed by Representation ID
	resultsChan       chan dash.DownloadResult     // Channel for download results

	// Playback state
	sessionTimescale  uint64 // The timescale of the primary (video) content, used for the main playhead
	currentTargetTime uint64 // Media time in sessionTimescale units, the "virtual playhead"

	// Control
	ctx        context.Context
	cancel     context.CancelFunc
	dashClient *dash.Client
}

// SessionManager manages all active live stream sessions.
type SessionManager struct {
	mutex      sync.RWMutex
	sessions   map[string]*StreamSession
	logger     logger.Logger
	cfg        *channels.ChannelConfig
	dashClient *dash.Client
	segCache   *cache.SegmentCache
}

// NewManager creates a new session manager.
func NewManager(log logger.Logger, cfg *channels.ChannelConfig, dashClient *dash.Client) *SessionManager {
	sm := &SessionManager{
		sessions:   make(map[string]*StreamSession),
		logger:     log,
		cfg:        cfg,
		dashClient: dashClient,
	}
	sm.segCache = cache.New(log, sm.GetAllActiveSegmentKeys)
	return sm
}

// Start begins the background workers for the manager's components.
func (sm *SessionManager) Start() {
	sm.segCache.Start()
}

// Stop gracefully shuts down all sessions and background workers.
func (sm *SessionManager) Stop() {
	sm.logger.Infof("Stopping session manager and all active sessions...")
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for _, session := range sm.sessions {
		session.Stop()
	}
	sm.segCache.Stop()
	sm.logger.Infof("Session manager stopped.")
}

// GetOrCreateSession retrieves an existing session or creates a new one.
func (sm *SessionManager) GetOrCreateSession(channelId string) (*StreamSession, error) {
	sm.mutex.RLock()
	session, found := sm.sessions[channelId]
	sm.mutex.RUnlock()

	if found {
		return session, nil
	}

	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	if session, found = sm.sessions[channelId]; found {
		return session, nil
	}

	sm.logger.Infof("No session found for channel ID: %s. Creating a new one.", channelId)

	var channelCfg *channels.Channel
	for i := range sm.cfg.Channels {
		if sm.cfg.Channels[i].Id == channelId {
			channelCfg = &sm.cfg.Channels[i]
			break
		}
	}

	if channelCfg == nil {
		return nil, fmt.Errorf("configuration for channel ID '%s' not found", channelId)
	}

	mpd, finalUrl, err := sm.dashClient.FetchAndParseMPD(channelCfg.ManifestURL, sm.cfg.UserAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to perform initial MPD fetch for channel '%s': %w", channelId, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	newSession := &StreamSession{
		ChannelID:         channelId,
		ManifestURL:       channelCfg.ManifestURL,
		BaseURL:           finalUrl,
		Logger:            sm.logger,
		MPD:               mpd,
		Downloader:        dash.NewDownloader(sm.dashClient.HttpClient(), sm.logger, sm.cfg.UserAgent, 10), // 10 concurrent workers
		SegCache:          sm.segCache,
		dashClient:        sm.dashClient, // Pass the client to the session
		availableSegments: make(map[string][]*models.Segment),
		playlistCache:     make(map[string]string),
		mediaSequence:     make(map[string]int),
		resultsChan:       make(chan dash.DownloadResult, 100),
		ctx:               ctx,
		cancel:            cancel,
	}

	if err := newSession.initializeState(); err != nil {
		return nil, fmt.Errorf("failed to initialize session state for channel '%s': %w", channelId, err)
	}

	sm.sessions[channelId] = newSession
	newSession.Start()
	sm.logger.Infof("Successfully created and started new session for channel: %s (%s)", channelCfg.Name, channelId)

	return newSession, nil
}

// downloadInitialSegments queues the download for the initialization segment for all selected representations.
func (s *StreamSession) downloadInitialSegments() {
	s.Logger.Infof("Queueing initialization segments for session %s...", s.ChannelID)

	for i := range s.MPD.Periods {
		period := &s.MPD.Periods[i]
		for j := range period.Sets {
			as := &period.Sets[j]
			repsToDownload := selectRepresentations(as)

			for _, rep := range repsToDownload {
				initURL, err := dash.BuildInitSegmentURL(s.BaseURL, period, as, rep)
				if err != nil {
					s.Logger.Warnf("Failed to build init segment URL for rep %s: %v", rep.ID, err)
					continue
				}

				cacheKey := fmt.Sprintf("%s/%s/init", s.ChannelID, rep.ID)
				if _, found := s.SegCache.Get(cacheKey); found {
					s.Logger.Debugf("Init segment for rep %s already in cache.", rep.ID)
					continue
				}

				s.Logger.Debugf("Queueing init segment for rep %s from %s", rep.ID, initURL)
				initSeg := models.Segment{URL: initURL, ID: cacheKey, RepID: rep.ID, IsInit: true}
				s.Downloader.QueueDownload(dash.DownloadTask{
					Segment: initSeg,
					Result:  s.resultsChan,
				})
			}
		}
	}
}

// Start kicks off the background goroutines for the session.
func (s *StreamSession) Start() {
	s.Logger.Infof("Starting background loops for session %s", s.ChannelID)
	s.downloadInitialSegments() // Queue init segments before starting loops
	go s.downloadLoop()
	go s.playlistLoop()
	go s.mpdRefreshLoop()
	go s.resultLoop()
}

// Stop terminates the background goroutines for the session.
func (s *StreamSession) Stop() {
	s.Logger.Infof("Stopping background loops for session %s", s.ChannelID)
	s.cancel()
	s.Downloader.Stop()
}

// downloadLoop is the "producer" goroutine.
func (s *StreamSession) downloadLoop() {
	ticker := time.NewTicker(2 * time.Second) // Check for new segments every 2s
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.Logger.Infof("Download loop for %s stopped.", s.ChannelID)
			return
		case <-ticker.C:
			s.downloadNextSegments()
		}
	}
}

func (s *StreamSession) initializeState() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var videoAS *dash.AdaptationSet
	// Find the primary video adaptation set to use as the master clock
	if len(s.MPD.Periods) > 0 {
		for i, as := range s.MPD.Periods[0].Sets {
			// Heuristic to find the main video content, excluding trick mode tracks
			if as.ContentType == "video" {
				isTrickMode := false
				for _, rep := range as.Representations {
					if strings.Contains(rep.ID, "TrickMode") {
						isTrickMode = true
						break
					}
				}
				if !isTrickMode {
					videoAS = &s.MPD.Periods[0].Sets[i]
					break
				}
			}
		}
	}

	if videoAS == nil {
		// Fallback to the first adaptation set if no suitable video is found
		if len(s.MPD.Periods) > 0 && len(s.MPD.Periods[0].Sets) > 0 {
			videoAS = &s.MPD.Periods[0].Sets[0]
			s.Logger.Warnf("No primary video adaptation set found, using first available set ('%s') for timing.", videoAS.ID)
		} else {
			return fmt.Errorf("no adaptation sets found in MPD")
		}
	}

	s.sessionTimescale = uint64(videoAS.SegmentTemplate.Timescale)
	if s.sessionTimescale == 0 {
		return fmt.Errorf("primary adaptation set has a timescale of 0")
	}

	timeline := videoAS.SegmentTemplate.Timeline.Segments
	if len(timeline) == 0 {
		return fmt.Errorf("primary adaptation set has no timeline information")
	}

	var maxTime uint64 = 0
	var timeCursor uint64 = 0
	for _, seg := range timeline {
		if seg.T > 0 {
			timeCursor = seg.T
		}
		timeCursor += uint64(seg.R+1) * seg.D
	}
	maxTime = timeCursor
	lastSegmentDuration := timeline[len(timeline)-1].D

	// Use a conservative live delay
	liveDelay := lastSegmentDuration * 4
	playhead := maxTime
	if playhead > liveDelay {
		playhead -= liveDelay
	} else {
		playhead = 0
	}
	s.currentTargetTime = playhead

	s.Logger.Infof("Initialized session state. Session timescale: %d (from AdaptationSet %s). Initial playhead time: %d", s.sessionTimescale, videoAS.ID, s.currentTargetTime)
	return nil
}

func (s *StreamSession) downloadNextSegments() {
	s.mutex.RLock()
	targetTime := s.currentTargetTime
	sessionTimescale := s.sessionTimescale
	mpd := s.MPD
	s.mutex.RUnlock()

	if sessionTimescale == 0 {
		s.Logger.Errorf("Session timescale is 0, cannot download segments.")
		time.Sleep(2 * time.Second) // Avoid busy-looping if state is bad
		return
	}

	var videoSegmentDuration uint64

	for i := range mpd.Periods {
		period := &mpd.Periods[i]
		for j := range period.Sets {
			as := &period.Sets[j]
			repsToDownload := selectRepresentations(as)

			repTimescale := uint64(as.SegmentTemplate.Timescale)
			if repTimescale == 0 {
				s.Logger.Warnf("Skipping AdaptationSet with ID %s because its timescale is 0", as.ID)
				continue
			}

			if len(repsToDownload) == 0 {
				continue
			}

			periodStart, err := period.GetStart()
			if err != nil {
				s.Logger.Warnf("Invalid period start time for period %s: %v", period.ID, err)
				continue
			}

			// Use the first representation for time calculations, assuming all are aligned.
			firstRep := repsToDownload[0]
			presentationTimeOffsetInSeconds := float64(firstRep.PresentationTimeOffset) / float64(repTimescale)
			presentationTimeInSeconds := float64(targetTime) / float64(sessionTimescale)
			periodStartInSeconds := periodStart.Seconds()

			mediaTimeInSeconds := presentationTimeInSeconds - periodStartInSeconds + presentationTimeOffsetInSeconds
			targetTimeForRep := uint64(mediaTimeInSeconds * float64(repTimescale))

			targetSegmentTime, targetSegmentDuration := findSegmentTimeForPlayhead(as.SegmentTemplate.Timeline, targetTimeForRep)

			if targetSegmentDuration == 0 {
				s.Logger.Debugf("No segment found for time %d in AdaptationSet %s", targetTimeForRep, as.ID)
				continue
			}

			if as.ContentType == "video" {
				videoSegmentDuration = targetSegmentDuration
			}

			for _, rep := range repsToDownload {
				segmentID := fmt.Sprintf("%d", targetSegmentTime)
				cacheKey := fmt.Sprintf("%s/%s/%s", s.ChannelID, rep.ID, segmentID)

				if _, found := s.SegCache.Get(cacheKey); found {
					continue // Already downloaded or in queue
				}

				segmentURL, err := dash.BuildSegmentURL(s.BaseURL, period, as, rep, targetSegmentTime)
				if err != nil {
					s.Logger.Warnf("Failed to build segment URL for time %d: %v", targetSegmentTime, err)
					continue
				}

				segment := models.Segment{
					URL:      segmentURL,
					ID:       cacheKey,
					Time:     targetSegmentTime,
					Duration: targetSegmentDuration,
					RepID:    rep.ID,
				}

				s.Logger.Debugf("Queueing media segment for rep %s, time %d", rep.ID, targetSegmentTime)
				s.Downloader.QueueDownload(dash.DownloadTask{
					Segment: segment,
					Result:  s.resultsChan,
				})
			}
		}
	}

	if videoSegmentDuration > 0 {
		s.mutex.Lock()
		s.currentTargetTime += videoSegmentDuration
		s.mutex.Unlock()
		s.Logger.Debugf("Advanced session playhead by %d to %d", videoSegmentDuration, s.currentTargetTime)
	}
}

// selectRepresentations applies the stream selection logic from the design document.
func selectRepresentations(as *dash.AdaptationSet) []*dash.Representation {
	var selected []*dash.Representation

	switch as.ContentType {
	case "video":
		var bestRep *dash.Representation
		maxBandwidth := 0
		for i := range as.Representations {
			rep := &as.Representations[i]
			// A simple way to identify and exclude trick mode tracks.
			// A more robust method might check for specific roles or other metadata.
			if strings.Contains(rep.ID, "TrickMode") {
				continue
			}
			if rep.Bandwidth > maxBandwidth {
				maxBandwidth = rep.Bandwidth
				bestRep = rep
			}
		}
		if bestRep != nil {
			selected = append(selected, bestRep)
		}
	case "audio", "text":
		// Select all available audio and text tracks
		for i := range as.Representations {
			selected = append(selected, &as.Representations[i])
		}
	}
	return selected
}

// findSegmentTimeForPlayhead finds the time and duration of the segment for the current playhead.
func findSegmentTimeForPlayhead(timeline dash.SegmentTimeline, playheadTime uint64) (uint64, uint64) {
	var timeCursor uint64 = 0
	for _, s := range timeline.Segments {
		// If a 't' attribute is present, the timeline resets to this value.
		if s.T > 0 {
			timeCursor = s.T
		}

		// There are s.R repeats, so s.R+1 segments in this block
		for i := 0; i <= s.R; i++ {
			segmentStartTime := timeCursor
			// The playhead falls within this segment's duration
			if playheadTime < segmentStartTime+s.D {
				return segmentStartTime, s.D
			}
			timeCursor += s.D
		}
	}

	// If playhead is past the known timeline, it means we are at the live edge.
	// Return the last known segment's start time and duration.
	if len(timeline.Segments) > 0 {
		// The timeCursor is now at the end of the timeline, so the last segment started one duration ago.
		lastDuration := timeline.Segments[len(timeline.Segments)-1].D
		if timeCursor > lastDuration {
			return timeCursor - lastDuration, lastDuration
		}
	}

	return 0, 0 // Should not happen with a valid timeline
}

// playlistLoop is the "publisher" goroutine.
func (s *StreamSession) playlistLoop() {
	ticker := time.NewTicker(1 * time.Second) // Regenerate playlist every second
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.Logger.Infof("Playlist loop for %s stopped.", s.ChannelID)
			return
		case <-ticker.C:
			s.updatePlaylists()
		}
	}
}

func (s *StreamSession) updatePlaylists() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, period := range s.MPD.Periods {
		for _, as := range period.Sets {
			for _, rep := range as.Representations {
				availableSegs := s.availableSegments[rep.ID]
				if len(availableSegs) == 0 {
					continue
				}

				// Keep only the last few segments for the live playlist
				if len(availableSegs) > playlistLiveSegments {
					availableSegs = availableSegs[len(availableSegs)-playlistLiveSegments:]
				}

				playlist, err := hls.GenerateMediaPlaylist(s.MPD, s.ChannelID, as.ContentType, rep.ID, s.mediaSequence[rep.ID], availableSegs)
				if err != nil {
					s.Logger.Warnf("Failed to generate media playlist for rep %s: %v", rep.ID, err)
					continue
				}
				s.playlistCache[rep.ID] = playlist
			}
		}
	}
}

// GetMasterPlaylist returns the master playlist.
func (s *StreamSession) GetMasterPlaylist() (string, error) {
	selectedReps := make(map[string][]*dash.Representation)
	for _, period := range s.MPD.Periods {
		for _, as := range period.Sets {
			reps := selectRepresentations(&as)
			if len(reps) > 0 {
				if _, ok := selectedReps[as.ContentType]; !ok {
					selectedReps[as.ContentType] = make([]*dash.Representation, 0)
				}
				selectedReps[as.ContentType] = append(selectedReps[as.ContentType], reps...)
			}
		}
	}
	return hls.GenerateMasterPlaylist(s.MPD, selectedReps)
}

// GetMediaPlaylist returns a media playlist from the cache.
func (s *StreamSession) GetMediaPlaylist(mediaType, repId string) (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	playlist, found := s.playlistCache[repId]
	if !found {
		return "", fmt.Errorf("playlist for representation %s not found in cache", repId)
	}
	return playlist, nil
}

// GetAllActiveSegmentKeys iterates through all sessions and collects the keys of all available segments,
// including init segments, to prevent them from being evicted.
func (sm *SessionManager) GetAllActiveSegmentKeys() map[string]struct{} {
	activeKeys := make(map[string]struct{})
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	for _, session := range sm.sessions {
		session.mutex.RLock()

		// Add active media segments
		for repId, segments := range session.availableSegments {
			for _, seg := range segments {
				// This is the correct cache key format for media segments
				cacheKey := fmt.Sprintf("%s/%s/%s", session.ChannelID, repId, seg.ID)
				activeKeys[cacheKey] = struct{}{}
			}
		}

		// Also add all init segments for all representations in the manifest
		if session.MPD != nil {
			for _, period := range session.MPD.Periods {
				for _, as := range period.Sets {
					for _, rep := range as.Representations {
						// This is the standardized cache key for init segments
						initKey := fmt.Sprintf("%s/%s/init", session.ChannelID, rep.ID)
						activeKeys[initKey] = struct{}{}
					}
				}
			}
		}

		session.mutex.RUnlock()
	}
	return activeKeys
}

// mpdRefreshLoop is a background goroutine that periodically fetches a new MPD.
func (s *StreamSession) mpdRefreshLoop() {
	// Determine the refresh interval
	refreshInterval := 5 * time.Second // A sensible default
	if s.MPD.MinimumUpdatePeriod != "" {
		if d, err := s.MPD.GetMinimumUpdatePeriod(); err == nil {
			refreshInterval = d
			// Per DASH spec, don't refresh more than every 2 seconds to avoid hammering the server
			if refreshInterval < 2*time.Second {
				refreshInterval = 2 * time.Second
			}
		} else {
			s.Logger.Warnf("Could not parse MinimumUpdatePeriod '%s', using default %v", s.MPD.MinimumUpdatePeriod, refreshInterval)
		}
	}
	s.Logger.Infof("Starting MPD refresh loop for session %s with interval %v", s.ChannelID, refreshInterval)
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.Logger.Infof("MPD refresh loop for %s stopped.", s.ChannelID)
			return
		case <-ticker.C:
			s.refreshMPD()
		}
	}
}

func (s *StreamSession) refreshMPD() {
	s.Logger.Debugf("Refreshing MPD for session %s from %s", s.ChannelID, s.ManifestURL)
	newMpd, newBaseURL, err := s.dashClient.FetchAndParseMPD(s.ManifestURL, "") // User agent is already in the client
	if err != nil {
		s.Logger.Warnf("Failed to refresh MPD for session %s: %v", s.ChannelID, err)
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Instead of replacing the whole MPD, merge the timelines
	for i := range newMpd.Periods {
		newPeriod := &newMpd.Periods[i]
		for j := range newPeriod.Sets {
			newAS := &newPeriod.Sets[j]

			// Find the corresponding old adaptation set
			var oldAS *dash.AdaptationSet
			for k := range s.MPD.Periods {
				if s.MPD.Periods[k].ID == newPeriod.ID {
					for l := range s.MPD.Periods[k].Sets {
						if s.MPD.Periods[k].Sets[l].ID == newAS.ID {
							oldAS = &s.MPD.Periods[k].Sets[l]
							break
						}
					}
				}
				if oldAS != nil {
					break
				}
			}

			if oldAS != nil {
				// Merge the timelines
				mergedTimeline := dash.MergeTimelines(oldAS.SegmentTemplate.Timeline, newAS.SegmentTemplate.Timeline)
				// Update the timeline in the session's MPD object
				oldAS.SegmentTemplate.Timeline = mergedTimeline
			} else {
				// This is a new AdaptationSet, we might need to add it.
				// For now, we'll log it. A more robust implementation would handle adding new periods/sets.
				s.Logger.Infof("Found new AdaptationSet with ID %s in refreshed MPD.", newAS.ID)
			}
		}
	}

	// Update other top-level attributes that might change
	s.MPD.MinimumUpdatePeriod = newMpd.MinimumUpdatePeriod
	s.BaseURL = newBaseURL
	s.Logger.Infof("Successfully refreshed and merged MPD for session %s", s.ChannelID)
}

// resultLoop is a background goroutine that processes download results.
func (s *StreamSession) resultLoop() {
	s.Logger.Infof("Starting result processing loop for session %s", s.ChannelID)
	for result := range s.resultsChan {
		if result.Error != nil {
			s.Logger.Warnf("Failed to download segment %s: %v", result.Task.Segment.ID, result.Error)
			continue
		}

		// The segment ID is the cache key
		cacheKey := result.Task.Segment.ID
		repID := result.Task.Segment.RepID

		s.SegCache.Set(cacheKey, result.Data)

		if result.Task.Segment.IsInit {
			s.Logger.Infof("Successfully downloaded and cached init segment for rep %s", repID)
		} else {
			s.mutex.Lock()
			// Create a copy of the segment to store in the session
			segCopy := result.Task.Segment
			// The ID for availableSegments should be the time, not the cache key
			segCopy.ID = fmt.Sprintf("%d", segCopy.Time)

			// Check for duplicates before appending
			found := false
			for _, existingSeg := range s.availableSegments[repID] {
				if existingSeg.ID == segCopy.ID {
					found = true
					break
				}
			}
			if !found {
				s.availableSegments[repID] = append(s.availableSegments[repID], &segCopy)
				if len(s.availableSegments[repID]) > playlistLiveSegments+2 {
					s.availableSegments[repID] = s.availableSegments[repID][1:]
					s.mediaSequence[repID]++
				}
			}
			s.mutex.Unlock()
			s.Logger.Infof("Successfully downloaded and cached segment %s for rep %s", cacheKey, repID)
		}
	}
	s.Logger.Infof("Result processing loop for %s stopped.", s.ChannelID)
}

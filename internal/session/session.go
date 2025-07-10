package session

import (
	"context"
	"dash2hlsd/internal/cache"
	"dash2hlsd/internal/config"
	"dash2hlsd/internal/dash"
	"dash2hlsd/internal/hls"
	"dash2hlsd/internal/logger"
	"dash2hlsd/internal/models"
	"fmt"
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
	Logger      logger.Logger
	MPD         *dash.MPD
	Downloader  *dash.SegmentDownloader
	SegCache    *cache.SegmentCache

	// Thread-safe state
	mutex             sync.RWMutex
	availableSegments map[string][]*models.Segment // Keyed by Representation ID
	playlistCache     map[string]string            // Keyed by Representation ID
	mediaSequence     map[string]int               // Keyed by Representation ID

	// Control
	ctx    context.Context
	cancel context.CancelFunc
}

// SessionManager manages all active live stream sessions.
type SessionManager struct {
	mutex      sync.RWMutex
	sessions   map[string]*StreamSession
	logger     logger.Logger
	cfg        *config.ChannelConfig
	dashClient *dash.Client
	segCache   *cache.SegmentCache
}

// NewManager creates a new session manager.
func NewManager(log logger.Logger, cfg *config.ChannelConfig, dashClient *dash.Client) *SessionManager {
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

	var channelCfg *config.Channel
	for i := range sm.cfg.Channels {
		if sm.cfg.Channels[i].Id == channelId {
			channelCfg = &sm.cfg.Channels[i]
			break
		}
	}

	if channelCfg == nil {
		return nil, fmt.Errorf("configuration for channel ID '%s' not found", channelId)
	}

	mpd, err := sm.dashClient.FetchAndParseMPD(channelCfg.ManifestURL, sm.cfg.UserAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to perform initial MPD fetch for channel '%s': %w", channelId, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	newSession := &StreamSession{
		ChannelID:         channelId,
		ManifestURL:       channelCfg.ManifestURL,
		Logger:            sm.logger,
		MPD:               mpd,
		Downloader:        dash.NewSegmentDownloader(sm.dashClient.HttpClient(), sm.logger, sm.cfg.UserAgent),
		SegCache:          sm.segCache,
		availableSegments: make(map[string][]*models.Segment),
		playlistCache:     make(map[string]string),
		mediaSequence:     make(map[string]int),
		ctx:               ctx,
		cancel:            cancel,
	}
	sm.sessions[channelId] = newSession
	newSession.Start()
	sm.logger.Infof("Successfully created and started new session for channel: %s (%s)", channelCfg.Name, channelId)

	return newSession, nil
}

// Start kicks off the background goroutines for the session.
func (s *StreamSession) Start() {
	s.Logger.Infof("Starting background loops for session %s", s.ChannelID)
	go s.downloadLoop()
	go s.playlistLoop()
}

// Stop terminates the background goroutines for the session.
func (s *StreamSession) Stop() {
	s.Logger.Infof("Stopping background loops for session %s", s.ChannelID)
	s.cancel()
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
			s.fetchLatestSegments()
		}
	}
}

func (s *StreamSession) fetchLatestSegments() {
	// In a real scenario, we would refresh the MPD periodically.
	// For now, we work with the initial MPD.
	var wg sync.WaitGroup
	for _, period := range s.MPD.Periods {
		for _, as := range period.Sets {
			// Apply stream selection logic (e.g., highest bitrate video)
			// For simplicity, we download for all representations for now.
			for _, rep := range as.Representations {
				timeline, err := dash.ConvertTimeline(s.ManifestURL, &as, &rep)
				if err != nil {
					s.Logger.Errorf("Error converting timeline for rep %s: %v", rep.ID, err)
					continue
				}

				if len(timeline) == 0 {
					continue
				}

				// Find the latest segment that we don't have yet.
				// This is a simplified logic. A robust implementation would manage a "virtual playhead".
				latestSegment := timeline[len(timeline)-1]

				s.mutex.RLock()
				found := false
				for _, seg := range s.availableSegments[rep.ID] {
					if seg.ID == latestSegment.ID {
						found = true
						break
					}
				}
				s.mutex.RUnlock()

				if !found {
					wg.Add(1)
					go func(segment models.Segment, repId string) {
						defer wg.Done()
						data, err := s.Downloader.DownloadSegment(segment)
						if err != nil {
							s.Logger.Warnf("Failed to download segment %s: %v", segment.ID, err)
							return
						}

						// Add to cache and available list
						s.SegCache.Set(segment.URL, data)
						s.mutex.Lock()
						s.availableSegments[repId] = append(s.availableSegments[repId], &segment)
						// Keep the list of available segments from growing indefinitely
						if len(s.availableSegments[repId]) > playlistLiveSegments+2 {
							s.availableSegments[repId] = s.availableSegments[repId][1:]
							s.mediaSequence[repId]++
						}
						s.mutex.Unlock()
						s.Logger.Infof("Successfully downloaded and cached segment %s for rep %s", segment.ID, repId)
					}(latestSegment, rep.ID)
				}
			}
		}
	}
	wg.Wait()
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
	return hls.GenerateMasterPlaylist(s.MPD, s.ChannelID)
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

// GetAllActiveSegmentKeys iterates through all sessions and collects the keys of all available segments.
func (sm *SessionManager) GetAllActiveSegmentKeys() map[string]struct{} {
	activeKeys := make(map[string]struct{})
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	for _, session := range sm.sessions {
		session.mutex.RLock()
		for _, segments := range session.availableSegments {
			for _, seg := range segments {
				activeKeys[seg.URL] = struct{}{}
			}
		}
		session.mutex.RUnlock()
	}
	return activeKeys
}

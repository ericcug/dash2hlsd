package api

import (
	"dash2hlsd/internal/key"
	"dash2hlsd/internal/session"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	playlistRetryInterval = 500 * time.Millisecond
	playlistMaxRetries    = 65 // 32.5 seconds total wait time, to accommodate downloader retries
)

type API struct {
	sessionMgr *session.SessionManager
	keyService *key.Service
}

func New(sessionMgr *session.SessionManager, keyService *key.Service) http.Handler {
	api := &API{
		sessionMgr: sessionMgr,
		keyService: keyService,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /live/{channelId}/master.m3u8", api.handleMasterPlaylist)
	mux.HandleFunc("GET /live/{channelId}/{mediaType}/{representationId}/playlist.m3u8", api.handleMediaPlaylist)
	mux.HandleFunc("GET /live/{channelId}/{mediaType}/{representationId}/{segmentName}", api.handleSegment)
	mux.HandleFunc("GET /key/{channelId}", api.handleKey)

	return mux
}

func (a *API) handleMasterPlaylist(w http.ResponseWriter, r *http.Request) {
	channelId := r.PathValue("channelId")
	sess, err := a.sessionMgr.GetOrCreateSession(channelId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get session: %v", err), http.StatusInternalServerError)
		return
	}

	playlist, err := sess.GetMasterPlaylist()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate master playlist: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Write([]byte(playlist))
}

func (a *API) handleMediaPlaylist(w http.ResponseWriter, r *http.Request) {
	channelId := r.PathValue("channelId")
	mediaType := r.PathValue("mediaType")
	repId := r.PathValue("representationId")

	sess, err := a.sessionMgr.GetOrCreateSession(channelId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get session: %v", err), http.StatusInternalServerError)
		return
	}

	var playlist string
	for i := 0; i < playlistMaxRetries; i++ {
		playlist, err = sess.GetMediaPlaylist(mediaType, repId)
		if err == nil {
			break // Success
		}
		sess.Logger.Debugf("Attempt %d: Media playlist for repId '%s' not ready, retrying in %v...", i+1, repId, playlistRetryInterval)
		time.Sleep(playlistRetryInterval)
	}

	if err != nil {
		sess.Logger.Errorf("Failed to generate media playlist for repId '%s' after %d attempts: %v. Returning 404.", repId, playlistMaxRetries, err)
		http.Error(w, fmt.Sprintf("Failed to generate media playlist: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Write([]byte(playlist))
}

func (a *API) handleSegment(w http.ResponseWriter, r *http.Request) {
	channelId := r.PathValue("channelId")
	repId := r.PathValue("representationId")
	segmentName := r.PathValue("segmentName") // This will always have .m4s suffix

	sess, err := a.sessionMgr.GetOrCreateSession(channelId)
	if err != nil {
		http.Error(w, "Failed to get session", http.StatusInternalServerError)
		return
	}

	// The logic is now extremely simple, as per your design.
	// We just construct the standardized cache key and look it up.
	segmentId := strings.TrimSuffix(segmentName, ".m4s")
	if segmentId == "init" {
		// This is the standardized name for init segments in the cache.
	}

	cacheKey := fmt.Sprintf("%s/%s/%s", channelId, repId, segmentId)

	sess.Logger.Debugf("Looking for segment in cache with key: %s", cacheKey)
	data, found := sess.SegCache.Get(cacheKey)
	if !found {
		http.Error(w, fmt.Sprintf("Segment %s not found in cache with key %s", segmentName, cacheKey), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Write(data)
}

func (a *API) handleKey(w http.ResponseWriter, r *http.Request) {
	channelId := r.PathValue("channelId")
	key, found := a.keyService.GetKeyForChannel(channelId)
	if !found {
		http.Error(w, "Key not found for the given channel", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(key)
}

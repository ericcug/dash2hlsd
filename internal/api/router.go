package api

import (
	"dash2hlsd/internal/key"
	"dash2hlsd/internal/session"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
	mux.HandleFunc("GET /live/{channelId}/{mediaType}/{representationId}/{fragmentId}.m4s", api.handleSegment)
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

	playlist, err := sess.GetMediaPlaylist(mediaType, repId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate media playlist: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Write([]byte(playlist))
}

func (a *API) handleSegment(w http.ResponseWriter, r *http.Request) {
	channelId := r.PathValue("channelId")
	mediaType := r.PathValue("mediaType")
	repId := r.PathValue("representationId")
	fragmentId := r.PathValue("fragmentId")

	// The segment URL is the key in the cache. We need to reconstruct it.
	// This is a simplification. A more robust solution would pass the full segment URL
	// or have a more deterministic way to build the key.
	sess, err := a.sessionMgr.GetOrCreateSession(channelId)
	if err != nil {
		http.Error(w, "Failed to get session", http.StatusInternalServerError)
		return
	}

	// Reconstruct the segment URL to use as the cache key.
	// This assumes a certain URL structure, which should be made more robust.
	// For now, we find the base URL from the manifest URL.
	baseURL, _ := url.Parse(sess.ManifestURL)
	var segPath string
	for _, p := range sess.MPD.Periods {
		for _, as := range p.Sets {
			if as.ContentType == mediaType {
				for _, r := range as.Representations {
					if r.ID == repId {
						segPath = strings.Replace(as.SegmentTemplate.Media, "$RepresentationID$", repId, 1)
						segPath = strings.Replace(segPath, "$Time$", fragmentId, 1)
						break
					}
				}
			}
		}
	}
	if segPath == "" {
		http.Error(w, "Could not determine segment path", http.StatusNotFound)
		return
	}

	segmentURL := baseURL.ResolveReference(&url.URL{Path: segPath})

	data, found := sess.SegCache.Get(segmentURL.String())
	if !found {
		http.Error(w, fmt.Sprintf("Segment %s not found in cache", fragmentId), http.StatusNotFound)
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

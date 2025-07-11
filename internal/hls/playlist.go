package hls

import (
	"dash2hlsd/internal/dash"
	"dash2hlsd/internal/models"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
)

// GenerateMasterPlaylist creates the HLS master playlist string from the selected representations.
func GenerateMasterPlaylist(mpd *dash.MPD, selectedReps map[string][]*dash.Representation) (string, error) {
	var sb strings.Builder
	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:7\n")

	// Audio and Subtitle renditions
	audioGroupID := "audio"
	subtitleGroupID := "subtitles"

	if reps, ok := selectedReps["audio"]; ok {
		for _, rep := range reps {
			sb.WriteString(fmt.Sprintf("#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"%s\",NAME=\"%s\",DEFAULT=YES,AUTOSELECT=YES,LANGUAGE=\"%s\",URI=\"audio/%s/playlist.m3u8\"\n",
				audioGroupID, rep.ID, rep.ID, rep.ID))
		}
	}
	if reps, ok := selectedReps["text"]; ok {
		for _, rep := range reps {
			sb.WriteString(fmt.Sprintf("#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID=\"%s\",NAME=\"%s\",DEFAULT=NO,AUTOSELECT=YES,LANGUAGE=\"%s\",URI=\"text/%s/playlist.m3u8\"\n",
				subtitleGroupID, rep.ID, rep.ID, rep.ID))
		}
	}

	// Video renditions
	if reps, ok := selectedReps["video"]; ok {
		for _, rep := range reps {
			sb.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,CODECS=\"%s\"", rep.Bandwidth, rep.Codecs))
			if rep.Width > 0 && rep.Height > 0 {
				sb.WriteString(fmt.Sprintf(",RESOLUTION=%dx%d", rep.Width, rep.Height))
			}
			if rep.FrameRate != "" {
				sb.WriteString(fmt.Sprintf(",FRAME-RATE=%.3f", parseFrameRate(rep.FrameRate)))
			}
			// Associate audio and subtitles
			if _, ok := selectedReps["audio"]; ok {
				sb.WriteString(fmt.Sprintf(",AUDIO=\"%s\"", audioGroupID))
			}
			if _, ok := selectedReps["text"]; ok {
				sb.WriteString(fmt.Sprintf(",SUBTITLES=\"%s\"", subtitleGroupID))
			}
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf("video/%s/playlist.m3u8\n", rep.ID))
		}
	}

	return sb.String(), nil
}

// GenerateMediaPlaylist creates the HLS media playlist string.
// Note: availableSegments would be provided by the session's download loop.
func GenerateMediaPlaylist(mpd *dash.MPD, channelId, mediaType, repId string, mediaSequence int, availableSegments []*models.Segment) (string, error) {
	var sb strings.Builder

	// Find the target representation
	var targetRep *dash.Representation
	var initURL string
	for _, p := range mpd.Periods {
		for _, as := range p.Sets {
			if as.ContentType == mediaType {
				for _, r := range as.Representations {
					if r.ID == repId {
						targetRep = &r
						initURL = strings.Replace(as.SegmentTemplate.Initialization, "$RepresentationID$", r.ID, 1)
						break
					}
				}
			}
			if targetRep != nil {
				break
			}
		}
		if targetRep != nil {
			break
		}
	}

	if targetRep == nil {
		return "", fmt.Errorf("representation '%s' of type '%s' not found", repId, mediaType)
	}

	durationStr := strings.ToLower(strings.TrimPrefix(mpd.MaxSegmentDuration, "PT"))
	targetDuration, _ := time.ParseDuration(durationStr)

	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:7\n")
	sb.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", int(targetDuration.Seconds())))
	sb.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", mediaSequence))
	// Assuming SAMPLE-AES for fMP4, key URI needs to be constructed based on channelId
	sb.WriteString(fmt.Sprintf("#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"/key/%s\"\n", channelId))
	base := path.Base(initURL)
	hlsInitFilename := strings.TrimSuffix(base, path.Ext(base)) + ".m4s"
	// The URI in the playlist should be relative to the playlist itself.
	sb.WriteString(fmt.Sprintf("#EXT-X-MAP:URI=\"%s\"\n", hlsInitFilename))

	// This part is illustrative. The actual segment list will come from the session manager.
	for _, seg := range availableSegments {
		// The duration in MPD is in timescale units. We need to convert it to seconds for EXTINF.
		timescale := float64(mpd.Periods[0].Sets[0].SegmentTemplate.Timescale) // Simplified assumption
		durationInSeconds := float64(seg.Duration) / timescale
		sb.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", durationInSeconds))
		// Segment URL should also be relative to the master playlist.
		// Segment URL should also be relative to the playlist.
		segmentURI := fmt.Sprintf("%s.m4s", seg.ID)
		sb.WriteString(fmt.Sprintf("%s\n", segmentURI))
	}

	return sb.String(), nil
}

func parseFrameRate(fr string) float64 {
	parts := strings.Split(fr, "/")
	if len(parts) == 2 {
		num, _ := strconv.ParseFloat(parts[0], 64)
		den, _ := strconv.ParseFloat(parts[1], 64)
		if den != 0 {
			return num / den
		}
	}
	f, _ := strconv.ParseFloat(fr, 64)
	return f
}

package hls

import (
	"dash2hlsd/internal/dash"
	"dash2hlsd/internal/models"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// GenerateMasterPlaylist creates the HLS master playlist string from the MPD data.
func GenerateMasterPlaylist(mpd *dash.MPD, channelId string) (string, error) {
	var sb strings.Builder
	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:7\n")

	for _, period := range mpd.Periods {
		for _, as := range period.Sets {
			// For each representation, create a stream info entry
			for _, rep := range as.Representations {
				sb.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,CODECS=\"%s\"", rep.Bandwidth, rep.Codecs))
				if rep.Width > 0 && rep.Height > 0 {
					sb.WriteString(fmt.Sprintf(",RESOLUTION=%dx%d", rep.Width, rep.Height))
				}
				if rep.FrameRate != "" {
					sb.WriteString(fmt.Sprintf(",FRAME-RATE=%.3f", parseFrameRate(rep.FrameRate)))
				}
				sb.WriteString("\n")
				// URL to the media playlist for this specific representation
				sb.WriteString(fmt.Sprintf("%s/%s/%s/playlist.m3u8\n", channelId, as.ContentType, rep.ID))
			}
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

	targetDuration, _ := time.ParseDuration(mpd.MaxSegmentDuration)

	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:7\n")
	sb.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", int(targetDuration.Seconds())))
	sb.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", mediaSequence))
	// Assuming SAMPLE-AES for fMP4, key URI needs to be constructed based on channelId
	sb.WriteString(fmt.Sprintf("#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"/key/%s\",IV=0x... \n", channelId)) // IV needs to be handled
	sb.WriteString(fmt.Sprintf("#EXT-X-MAP:URI=\"%s/%s/%s/%s\"\n", channelId, mediaType, repId, initURL))

	// This part is illustrative. The actual segment list will come from the session manager.
	for _, seg := range availableSegments {
		// The duration in MPD is in timescale units. We need to convert it to seconds for EXTINF.
		timescale := float64(mpd.Periods[0].Sets[0].SegmentTemplate.Timescale) // Simplified assumption
		durationInSeconds := float64(seg.Duration) / timescale
		sb.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", durationInSeconds))
		sb.WriteString(fmt.Sprintf("%s/%s/%s/%s.m4s\n", channelId, mediaType, repId, seg.ID))
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

package dash

import (
	"dash2hlsd/internal/models"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ConvertTimeline processes the SegmentTimeline from an AdaptationSet and returns a flat list of all segments.
func ConvertTimeline(baseURL string, as *AdaptationSet, rep *Representation) ([]models.Segment, error) {
	var segments []models.Segment
	var currentTime uint64 = 0

	template := as.SegmentTemplate
	mediaURLTemplate := template.Media

	// Resolve the base URL for segments
	segmentBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	for _, s := range template.Timeline.Segments {
		// If t is specified, it's an absolute start time.
		if s.T > 0 {
			currentTime = s.T
		}

		// Handle a single segment
		seg := createSegment(segmentBaseURL, mediaURLTemplate, rep.ID, currentTime, s.D)
		segments = append(segments, seg)
		currentTime += s.D

		// Handle repeated segments (r attribute)
		// The r attribute specifies the number of following segments with the same duration.
		// r=-1 means it repeats until the end of the period. For live, we can treat it as a large number or handle it based on update logic.
		// For now, we handle explicit repeats.
		for i := 0; i < s.R; i++ {
			seg := createSegment(segmentBaseURL, mediaURLTemplate, rep.ID, currentTime, s.D)
			segments = append(segments, seg)
			currentTime += s.D
		}
	}

	return segments, nil
}

func createSegment(base *url.URL, mediaTemplate, repID string, time, duration uint64) models.Segment {
	// Replace placeholders in the media template
	mediaPath := strings.Replace(mediaTemplate, "$RepresentationID$", repID, 1)
	mediaPath = strings.Replace(mediaPath, "$Time$", strconv.FormatUint(time, 10), 1)

	// Resolve the segment path against the base URL
	segmentURL := base.ResolveReference(&url.URL{Path: mediaPath})

	return models.Segment{
		URL:      segmentURL.String(),
		ID:       strconv.FormatUint(time, 10),
		Time:     time,
		Duration: duration,
	}
}

package dash

import (
	"dash2hlsd/internal/logger"
	"dash2hlsd/internal/models"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// ConvertTimeline processes the SegmentTimeline from an AdaptationSet and returns a flat list of all segments.
// BuildSegment constructs a single segment's metadata, including its URL.
func BuildSegment(baseURL string, period *Period, as *AdaptationSet, rep *Representation, time, duration uint64, log logger.Logger) (models.Segment, error) {
	template := as.SegmentTemplate
	mediaURLTemplate := template.Media

	// Resolve the base URL for segments
	segmentBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return models.Segment{}, fmt.Errorf("invalid base URL: %w", err)
	}

	// If the period has its own BaseURL, resolve it against the main base URL
	if period.BaseURL != "" {
		periodBase, err := url.Parse(period.BaseURL)
		if err != nil {
			return models.Segment{}, fmt.Errorf("invalid period BaseURL: %w", err)
		}
		segmentBaseURL = segmentBaseURL.ResolveReference(periodBase)
	}

	// Replace placeholders in the media template
	mediaPath := strings.Replace(mediaURLTemplate, "$RepresentationID$", rep.ID, 1)
	mediaPath = strings.Replace(mediaPath, "$Time$", strconv.FormatUint(time, 10), 1)

	// Resolve the segment path against the base URL
	segmentURL := segmentBaseURL.ResolveReference(&url.URL{Path: mediaPath})

	log.Debugf("Constructed segment URL: %s", segmentURL.String())

	return models.Segment{
		URL:      segmentURL.String(),
		ID:       strconv.FormatUint(time, 10),
		Time:     time,
		Duration: duration,
	}, nil
}

// MergeTimelines combines two SegmentTimelines, removing duplicates and keeping it sorted.
func MergeTimelines(oldTimeline, newTimeline SegmentTimeline) SegmentTimeline {
	seen := make(map[uint64]S)

	// Add all segments from the old timeline
	for _, s := range oldTimeline.Segments {
		seen[s.T] = s
	}

	// Add all segments from the new timeline, overwriting duplicates
	// This assumes the new timeline is more up-to-date for any overlapping segments
	for _, s := range newTimeline.Segments {
		seen[s.T] = s
	}

	// Convert map back to slice
	merged := make([]S, 0, len(seen))
	for _, s := range seen {
		merged = append(merged, s)
	}

	// Sort the merged slice by start time 't'
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].T < merged[j].T
	})

	return SegmentTimeline{Segments: merged}
}

package dash

import (
	"encoding/xml"
	"errors"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MPD is the root element of a Media Presentation Description.
type MPD struct {
	XMLName               xml.Name `xml:"MPD"`
	Type                  string   `xml:"type,attr"`
	Profiles              string   `xml:"profiles,attr"`
	MinimumUpdatePeriod   string   `xml:"minimumUpdatePeriod,attr"`
	TimeShiftBufferDepth  string   `xml:"timeShiftBufferDepth,attr"`
	AvailabilityStartTime string   `xml:"availabilityStartTime,attr"`
	PublishTime           string   `xml:"publishTime,attr"`
	MaxSegmentDuration    string   `xml:"maxSegmentDuration,attr"`
	MinBufferTime         string   `xml:"minBufferTime,attr"`
	Periods               []Period `xml:"Period"`
}

// GetMinimumUpdatePeriod returns the MinimumUpdatePeriod as a time.Duration.
func (m *MPD) GetMinimumUpdatePeriod() (time.Duration, error) {
	return parseDuration(m.MinimumUpdatePeriod)
}

// parseDuration parses an ISO 8601 duration string like "PT8S".
func parseDuration(duration string) (time.Duration, error) {
	if !strings.HasPrefix(duration, "PT") {
		// Fallback for simple duration strings like "5s"
		return time.ParseDuration(duration)
	}

	duration = strings.TrimPrefix(duration, "PT")
	var totalDuration time.Duration
	re := regexp.MustCompile(`(\d+\.?\d*)(\w)`)
	matches := re.FindAllStringSubmatch(duration, -1)

	if len(matches) == 0 && duration != "" {
		// Handle cases like "PT" which means zero duration
		if duration == "" {
			return 0, nil
		}
		return 0, errors.New("invalid ISO 8601 duration format")
	}

	for _, match := range matches {
		valueStr := match[1]
		unit := match[2]

		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return 0, err
		}

		switch unit {
		case "H":
			totalDuration += time.Duration(value * float64(time.Hour))
		case "M":
			totalDuration += time.Duration(value * float64(time.Minute))
		case "S":
			totalDuration += time.Duration(value * float64(time.Second))
		default:
			return 0, errors.New("unsupported duration unit: " + unit)
		}
	}

	return totalDuration, nil
}

// Period represents a media content period.
type Period struct {
	ID      string          `xml:"id,attr"`
	Start   string          `xml:"start,attr"`
	BaseURL string          `xml:"BaseURL"`
	Sets    []AdaptationSet `xml:"AdaptationSet"`
}

// GetStart returns the Period's start time as a time.Duration.
func (p *Period) GetStart() (time.Duration, error) {
	if p.Start == "" {
		return 0, nil
	}
	return parseDuration(p.Start)
}

// AdaptationSet represents a set of interchangeable representations.
type AdaptationSet struct {
	ID               string           `xml:"id,attr"`
	ContentType      string           `xml:"contentType,attr"`
	Lang             string           `xml:"lang,attr,omitempty"`
	MimeType         string           `xml:"mimeType,attr"`
	SegmentAlignment bool             `xml:"segmentAlignment,attr"`
	StartWithSAP     int              `xml:"startWithSAP,attr"`
	MaxWidth         int              `xml:"maxWidth,attr,omitempty"`
	MaxHeight        int              `xml:"maxHeight,attr,omitempty"`
	Par              string           `xml:"par,attr,omitempty"`
	CodingDependency bool             `xml:"codingDependency,attr,omitempty"`
	Representations  []Representation `xml:"Representation"`
	SegmentTemplate  SegmentTemplate  `xml:"SegmentTemplate"`
}

// Representation represents a specific media stream.
type Representation struct {
	ID                     string `xml:"id,attr"`
	Bandwidth              int    `xml:"bandwidth,attr"`
	Codecs                 string `xml:"codecs,attr"`
	Width                  int    `xml:"width,attr,omitempty"`
	Height                 int    `xml:"height,attr,omitempty"`
	FrameRate              string `xml:"frameRate,attr,omitempty"`
	AudioSamplingRate      int    `xml:"audioSamplingRate,attr,omitempty"`
	PresentationTimeOffset uint64 `xml:"presentationTimeOffset,attr,omitempty"`
}

// SegmentTemplate defines the URL structure for segments.
type SegmentTemplate struct {
	Timescale      int             `xml:"timescale,attr"`
	Initialization string          `xml:"initialization,attr"`
	Media          string          `xml:"media,attr"`
	Timeline       SegmentTimeline `xml:"SegmentTimeline"`
}

// GetInitializationFilename extracts the filename from the Initialization attribute.
func (st *SegmentTemplate) GetInitializationFilename() string {
	return path.Base(st.Initialization)
}

// SegmentTimeline defines the timeline of segments.
type SegmentTimeline struct {
	Segments []S `xml:"S"`
}

// S represents a single segment or a series of segments.
type S struct {
	T uint64 `xml:"t,attr"`           // Start time
	D uint64 `xml:"d,attr"`           // Duration
	R int    `xml:"r,attr,omitempty"` // Repeat count
}

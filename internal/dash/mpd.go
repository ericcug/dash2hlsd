package dash

import "encoding/xml"

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

// Period represents a media content period.
type Period struct {
	ID      string          `xml:"id,attr"`
	Start   string          `xml:"start,attr"`
	BaseURL string          `xml:"BaseURL"`
	Sets    []AdaptationSet `xml:"AdaptationSet"`
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
	ID                string `xml:"id,attr"`
	Bandwidth         int    `xml:"bandwidth,attr"`
	Codecs            string `xml:"codecs,attr"`
	Width             int    `xml:"width,attr,omitempty"`
	Height            int    `xml:"height,attr,omitempty"`
	FrameRate         string `xml:"frameRate,attr,omitempty"`
	AudioSamplingRate int    `xml:"audioSamplingRate,attr,omitempty"`
}

// SegmentTemplate defines the URL structure for segments.
type SegmentTemplate struct {
	Timescale      int             `xml:"timescale,attr"`
	Initialization string          `xml:"initialization,attr"`
	Media          string          `xml:"media,attr"`
	Timeline       SegmentTimeline `xml:"SegmentTimeline"`
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

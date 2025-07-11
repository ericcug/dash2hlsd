package models

// Segment represents a media segment with its essential properties.
// This struct is used across different packages to represent a downloadable chunk of media.
type Segment struct {
	// URL is the fully-qualified URL to fetch the segment from.
	URL string
	// ID is a unique identifier for the segment, often derived from its start time or sequence number.
	// For media segments, this is the cache key.
	ID string
	// Time is the start time of the segment in the timescale of its representation.
	Time uint64
	// Duration is the duration of the segment in the timescale of its representation.
	Duration uint64
	// RepID is the ID of the representation this segment belongs to.
	RepID string
	// IsInit indicates if this is an initialization segment.
	IsInit bool
}

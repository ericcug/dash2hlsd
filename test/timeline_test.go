package main_test

import (
	"dash2hlsd/internal/dash"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeTimelines(t *testing.T) {
	t.Run("non-overlapping", func(t *testing.T) {
		oldTimeline := dash.SegmentTimeline{
			Segments: []dash.S{
				{T: 0, D: 10},
				{T: 10, D: 10},
			},
		}
		newTimeline := dash.SegmentTimeline{
			Segments: []dash.S{
				{T: 20, D: 10},
				{T: 30, D: 10},
			},
		}
		merged := dash.MergeTimelines(oldTimeline, newTimeline)
		assert.Len(t, merged.Segments, 4)
		assert.Equal(t, uint64(0), merged.Segments[0].T)
		assert.Equal(t, uint64(10), merged.Segments[1].T)
		assert.Equal(t, uint64(20), merged.Segments[2].T)
		assert.Equal(t, uint64(30), merged.Segments[3].T)
	})

	t.Run("overlapping", func(t *testing.T) {
		oldTimeline := dash.SegmentTimeline{
			Segments: []dash.S{
				{T: 0, D: 10},
				{T: 10, D: 10},
			},
		}
		newTimeline := dash.SegmentTimeline{
			Segments: []dash.S{
				{T: 10, D: 12}, // Overwrites old segment at T=10
				{T: 22, D: 10},
			},
		}
		merged := dash.MergeTimelines(oldTimeline, newTimeline)
		assert.Len(t, merged.Segments, 3)
		assert.Equal(t, uint64(0), merged.Segments[0].T)
		assert.Equal(t, uint64(10), merged.Segments[1].T)
		assert.Equal(t, uint64(12), merged.Segments[1].D, "Duration should be updated from new timeline")
		assert.Equal(t, uint64(22), merged.Segments[2].T)
	})

	t.Run("subset", func(t *testing.T) {
		oldTimeline := dash.SegmentTimeline{
			Segments: []dash.S{
				{T: 0, D: 10},
				{T: 10, D: 10},
				{T: 20, D: 10},
			},
		}
		newTimeline := dash.SegmentTimeline{
			Segments: []dash.S{
				{T: 10, D: 10},
			},
		}
		merged := dash.MergeTimelines(oldTimeline, newTimeline)
		assert.Len(t, merged.Segments, 3)
		assert.Equal(t, uint64(0), merged.Segments[0].T)
		assert.Equal(t, uint64(10), merged.Segments[1].T)
		assert.Equal(t, uint64(20), merged.Segments[2].T)
	})

	t.Run("empty old", func(t *testing.T) {
		oldTimeline := dash.SegmentTimeline{}
		newTimeline := dash.SegmentTimeline{
			Segments: []dash.S{
				{T: 10, D: 10},
			},
		}
		merged := dash.MergeTimelines(oldTimeline, newTimeline)
		assert.Len(t, merged.Segments, 1)
		assert.Equal(t, uint64(10), merged.Segments[0].T)
	})

	t.Run("empty new", func(t *testing.T) {
		oldTimeline := dash.SegmentTimeline{
			Segments: []dash.S{
				{T: 10, D: 10},
			},
		}
		newTimeline := dash.SegmentTimeline{}
		merged := dash.MergeTimelines(oldTimeline, newTimeline)
		assert.Len(t, merged.Segments, 1)
		assert.Equal(t, uint64(10), merged.Segments[0].T)
	})
}

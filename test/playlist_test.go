package main_test

import (
	"dash2hlsd/internal/dash"
	"dash2hlsd/internal/hls"
	"dash2hlsd/internal/models"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateMasterPlaylist(t *testing.T) {
	mpd := &dash.MPD{
		Periods: []dash.Period{
			{
				Sets: []dash.AdaptationSet{
					{
						ContentType: "video",
						Representations: []dash.Representation{
							{ID: "v1", Bandwidth: 5000000, Codecs: "avc1.640028", Width: 1920, Height: 1080, FrameRate: "25"},
							{ID: "v2", Bandwidth: 2000000, Codecs: "avc1.64001F", Width: 1280, Height: 720, FrameRate: "25/1"},
						},
					},
					{
						ContentType: "audio",
						Representations: []dash.Representation{
							{ID: "a1", Bandwidth: 128000, Codecs: "mp4a.40.2"},
						},
					},
				},
			},
		},
	}

	selectedReps := map[string][]*dash.Representation{
		"video": {&mpd.Periods[0].Sets[0].Representations[0], &mpd.Periods[0].Sets[0].Representations[1]},
		"audio": {&mpd.Periods[0].Sets[1].Representations[0]},
	}
	playlist, err := hls.GenerateMasterPlaylist(mpd, selectedReps)
	assert.NoError(t, err)

	// Check for video stream 1
	assert.Contains(t, playlist, "#EXT-X-STREAM-INF:BANDWIDTH=5000000,CODECS=\"avc1.640028\",RESOLUTION=1920x1080,FRAME-RATE=25.000")
	assert.Contains(t, playlist, "video/v1/playlist.m3u8")

	// Check for video stream 2
	assert.Contains(t, playlist, "#EXT-X-STREAM-INF:BANDWIDTH=2000000,CODECS=\"avc1.64001F\",RESOLUTION=1280x720,FRAME-RATE=25.000")
	assert.Contains(t, playlist, "video/v2/playlist.m3u8")

	// Check for audio stream
	assert.Contains(t, playlist, "#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\",NAME=\"a1\",DEFAULT=YES,AUTOSELECT=YES,LANGUAGE=\"a1\",URI=\"audio/a1/playlist.m3u8\"")
}

func TestGenerateMediaPlaylist(t *testing.T) {
	mpd := &dash.MPD{
		MaxSegmentDuration: "PT6S",
		Periods: []dash.Period{
			{
				Sets: []dash.AdaptationSet{
					{
						ContentType: "video",
						SegmentTemplate: dash.SegmentTemplate{
							Timescale:      90000,
							Initialization: "init-$RepresentationID$.m4s",
						},
						Representations: []dash.Representation{
							{ID: "v1", Bandwidth: 5000000, Codecs: "avc1.640028"},
						},
					},
				},
			},
		},
	}

	segments := []*models.Segment{
		{ID: "12345", Duration: 540000}, // 6 seconds * 90000 timescale
		{ID: "12351", Duration: 540000},
	}

	playlist, err := hls.GenerateMediaPlaylist(mpd, "test_channel", "video", "v1", 101, segments)
	assert.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(playlist), "\n")

	assert.Equal(t, "#EXTM3U", lines[0])
	assert.Equal(t, "#EXT-X-VERSION:7", lines[1])
	assert.Equal(t, "#EXT-X-TARGETDURATION:6", lines[2])
	assert.Equal(t, "#EXT-X-MEDIA-SEQUENCE:101", lines[3])
	assert.Equal(t, "#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"/key/test_channel\"", lines[4])
	assert.Equal(t, "#EXT-X-MAP:URI=\"init-v1.m4s\"", lines[5])

	// Check segment 1
	assert.Equal(t, "#EXTINF:6.000,", lines[6])
	assert.Equal(t, "12345.m4s", lines[7])

	// Check segment 2
	assert.Equal(t, "#EXTINF:6.000,", lines[8])
	assert.Equal(t, "12351.m4s", lines[9])
}

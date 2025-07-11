package main_test

import (
	"encoding/xml"
	"io/ioutil"
	"testing"

	"dash2hlsd/internal/dash"

	"github.com/stretchr/testify/assert"
)

func TestParsePearlMPD(t *testing.T) {
	xmlFile, err := ioutil.ReadFile("../docs/pearl.mpd")
	assert.NoError(t, err)

	var mpd dash.MPD
	err = xml.Unmarshal(xmlFile, &mpd)
	assert.NoError(t, err)

	assert.Equal(t, "dynamic", mpd.Type)
	assert.Equal(t, "urn:mpeg:dash:profile:isoff-live:2011,urn:dvb:dash:profile:dvb-dash:2014,urn:dvb:dash:profile:dvb-dash:isoff-ext-live:2014,http://dashif.org/guidelines/dash-if-simple", mpd.Profiles)
	assert.Equal(t, "PT12H0S", mpd.TimeShiftBufferDepth)
	assert.Equal(t, "PT8S", mpd.MinimumUpdatePeriod)
	assert.Equal(t, "PT12.00S", mpd.MaxSegmentDuration)
	assert.Equal(t, "1970-01-01T00:00:00Z", mpd.AvailabilityStartTime)
	assert.Equal(t, "2025-07-09T15:05:52Z", mpd.PublishTime)
	assert.Equal(t, "PT8S", mpd.MinBufferTime)

	assert.Len(t, mpd.Periods, 1)
	period := mpd.Periods[0]
	assert.Equal(t, "p_3_0", period.ID)
	assert.Equal(t, "PT0S", period.Start)
	assert.Equal(t, "3/", period.BaseURL)

	assert.Len(t, period.Sets, 7)

	// Video AdaptationSet
	videoSet := period.Sets[0]
	assert.Equal(t, "1", videoSet.ID)
	assert.Equal(t, "video", videoSet.ContentType)
	assert.Equal(t, "video/mp4", videoSet.MimeType)
	assert.Equal(t, 1920, videoSet.MaxWidth)
	assert.Equal(t, 1080, videoSet.MaxHeight)
	assert.Len(t, videoSet.Representations, 2)
	assert.Equal(t, "v5000000", videoSet.Representations[0].ID)
	assert.Equal(t, 5000000, videoSet.Representations[0].Bandwidth)
	assert.Equal(t, "v1500000", videoSet.Representations[1].ID)
	assert.Equal(t, 1500000, videoSet.Representations[1].Bandwidth)

	// Audio AdaptationSet (en)
	audioSetEn := period.Sets[2]
	assert.Equal(t, "3", audioSetEn.ID)
	assert.Equal(t, "audio", audioSetEn.ContentType)
	assert.Equal(t, "en", audioSetEn.Lang)
	assert.Equal(t, "audio/mp4", audioSetEn.MimeType)
	assert.Len(t, audioSetEn.Representations, 1)
	assert.Equal(t, "a128000", audioSetEn.Representations[0].ID)
	assert.Equal(t, 128000, audioSetEn.Representations[0].Bandwidth)

	// Subtitle AdaptationSet (zh)
	subtitleSetZh := period.Sets[4]
	assert.Equal(t, "5", subtitleSetZh.ID)
	assert.Equal(t, "text", subtitleSetZh.ContentType)
	assert.Equal(t, "zh", subtitleSetZh.Lang)
	assert.Equal(t, "application/mp4", subtitleSetZh.MimeType)
	assert.Len(t, subtitleSetZh.Representations, 1)
	assert.Equal(t, "s10000_chi", subtitleSetZh.Representations[0].ID)
	assert.Equal(t, 10000, subtitleSetZh.Representations[0].Bandwidth)
}

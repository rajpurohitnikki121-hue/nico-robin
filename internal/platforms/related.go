package platforms

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Laky-64/gologging"

	state "main/internal/core/models"
)

// RelatedTrack returns a track related to t, for autoplay. It uses the YouTube
// "Mix" (radio) of the track, so the next song matches the one that just
// played. Tracks whose ID is in skip (already played) are ignored, as are live
// streams and tracks longer than maxDuration seconds (0 = no limit).
//
// For tracks that do not come from YouTube the YouTube counterpart is looked
// up by title first.
func RelatedTrack(
	t *state.Track,
	skip map[string]bool,
	maxDuration int,
) (*state.Track, error) {
	if t == nil {
		return nil, errors.New("no current track")
	}

	videoID := ""
	if t.Source == PlatformYouTube {
		videoID = t.ID
	}
	if videoID == "" {
		res, err := yt.VideoSearch(t.Title, true)
		if err != nil || len(res) == 0 {
			return nil, fmt.Errorf("could not find %q on YouTube", t.Title)
		}
		videoID = res[0].ID
	}

	pick := func(list []*state.Track) *state.Track {
		for _, c := range list {
			if c == nil || c.ID == "" || c.ID == videoID || skip[c.ID] {
				continue
			}
			if c.Duration <= 0 || (maxDuration > 0 && c.Duration > maxDuration) {
				continue
			}
			cp := *c // do not modify the cached track
			cp.Video = false
			return &cp
		}
		return nil
	}

	// 1) the YouTube Mix (radio) of the song
	mix, mixErr := fetchRelatedMix(videoID, 25)
	if next := pick(mix); next != nil {
		return next, nil
	}

	// 2) fall back to a normal search for the title, so autoplay still goes on
	// when the Mix cannot be read
	if mixErr != nil {
		gologging.WarnF("[autoplay] mix failed (%v), using search", mixErr)
	}
	query := t.Title
	if i := strings.IndexAny(query, "|(["); i > 3 {
		query = query[:i]
	}
	query = strings.TrimSpace(query) + " songs"
	if res, err := yt.VideoSearch(query); err == nil {
		if next := pick(res); next != nil {
			return next, nil
		}
	}

	if mixErr != nil {
		return nil, fmt.Errorf("mix failed: %w", mixErr)
	}
	return nil, errors.New("no related track found")
}

// fetchRelatedMix reads the "RD<videoId>" Mix. Unlike a plain playlist call it
// also sends the video id, which YouTube needs to build the Mix reliably.
func fetchRelatedMix(videoID string, limit int) ([]*state.Track, error) {
	payload := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    innerTubeClientName,
				"clientVersion": innerTubeClientVersion,
				"hl":            "en-IN",
				"gl":            "IN",
			},
		},
		"videoId":    videoID,
		"playlistId": "RD" + videoID,
	}

	var result map[string]any
	if err := yt.callInnerTube("next", payload, &result); err != nil {
		return nil, err
	}

	items, ok := dig(
		result,
		"contents",
		"twoColumnWatchNextResults",
		"playlist",
		"playlist",
		"contents",
	).([]any)
	if !ok {
		return nil, errors.New("mix contents not found")
	}

	var tracks []*state.Track
	for _, item := range items {
		if limit > 0 && len(tracks) >= limit {
			break
		}
		vid, ok := dig(item, "playlistPanelVideoRenderer").(map[string]any)
		if !ok {
			continue
		}
		id := safeString(vid["videoId"])
		if id == "" {
			continue
		}
		tracks = append(tracks, &state.Track{
			URL:      "https://www.youtube.com/watch?v=" + id,
			Title:    safeString(dig(vid, "title", "simpleText")),
			ID:       id,
			Artwork:  getThumbnailURL(vid),
			Duration: parseDuration(safeString(dig(vid, "lengthText", "simpleText"))),
			Source:   PlatformYouTube,
		})
	}
	if len(tracks) == 0 {
		return nil, errors.New("mix is empty")
	}
	return tracks, nil
}

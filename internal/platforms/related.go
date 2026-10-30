package platforms

import (
	"errors"
	"fmt"

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

	mix, err := yt.fetchMixPlaylist("RD"+videoID, 25)
	if err != nil {
		return nil, err
	}

	for _, c := range mix {
		if c == nil || c.ID == "" || c.ID == videoID || skip[c.ID] {
			continue
		}
		if c.Duration <= 0 || (maxDuration > 0 && c.Duration > maxDuration) {
			continue
		}
		cp := *c // do not modify the cached track
		cp.Video = false
		return &cp, nil
	}

	return nil, errors.New("no related track found")
}

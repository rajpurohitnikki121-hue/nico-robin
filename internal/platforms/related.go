package platforms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os/exec"
	"strings"
	"time"
	"unicode"

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

	baseName := songBase(t.Title)
	pick := func(list []*state.Track) *state.Track {
		var ok []*state.Track
		for _, c := range list {
			if c == nil || c.ID == "" || c.ID == videoID || skip[c.ID] {
				continue
			}
			if c.Duration <= 0 || (maxDuration > 0 && c.Duration > maxDuration) {
				continue
			}
			// Another upload / version of the same song is not "related".
			if sameSong(baseName, songBase(c.Title)) {
				continue
			}
			ok = append(ok, c)
			if len(ok) >= 6 {
				break
			}
		}
		if len(ok) == 0 {
			return nil
		}
		cp := *ok[rand.Intn(len(ok))] // do not modify the cached track
		cp.Video = false
		return &cp
	}

	// 1) the YouTube Mix (radio) of the song
	mix, mixErr := fetchRelatedMix(videoID, 25)
	if next := pick(mix); next != nil {
		return next, nil
	}
	if mixErr != nil {
		gologging.WarnF("[autoplay] mix failed (%v), trying yt-dlp", mixErr)
	}

	// 2) the same Mix read through yt-dlp
	if list, err := ytdlpMix(videoID); err == nil {
		if next := pick(list); next != nil {
			return next, nil
		}
	} else {
		gologging.WarnF("[autoplay] yt-dlp mix failed (%v), using search", err)
	}

	// 3) a search for similar songs
	for _, q := range []string{
		"songs like " + baseName,
		baseName + " similar songs",
	} {
		if res, err := yt.VideoSearch(q); err == nil {
			if next := pick(res); next != nil {
				return next, nil
			}
		}
	}

	if mixErr != nil {
		return nil, fmt.Errorf("mix failed: %w", mixErr)
	}
	return nil, errors.New("no related track found")
}

var noiseWords = map[string]bool{
	"lyrical": true, "lyrics": true, "lyric": true, "video": true,
	"official": true, "song": true, "audio": true, "full": true, "hd": true,
	"4k": true, "version": true, "visualizer": true, "music": true,
	"new": true, "latest": true, "with": true,
}

// songBase reduces a video title to the song name: the part before the first
// "|", "(" or "[", lower case, without words like "lyrical" or "official".
func songBase(title string) string {
	if i := strings.IndexAny(title, "|([-"); i > 2 {
		title = title[:i]
	}
	var words []string
	for _, w := range strings.Fields(strings.ToLower(title)) {
		w = strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return -1
		}, w)
		if w != "" && !noiseWords[w] {
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}

func sameSong(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a == b || (len(a) >= 4 && strings.Contains(b, a)) ||
		(len(b) >= 4 && strings.Contains(a, b))
}

// ytdlpMix reads the "RD<videoId>" Mix with yt-dlp (flat, no downloads).
func ytdlpMix(videoID string) ([]*state.Track, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "yt-dlp",
		"--flat-playlist", "--no-warnings", "-J", "--playlist-end", "20",
		"https://www.youtube.com/watch?v="+videoID+"&list=RD"+videoID,
	).Output()
	if err != nil {
		return nil, err
	}

	var data struct {
		Entries []struct {
			ID       string  `json:"id"`
			Title    string  `json:"title"`
			Duration float64 `json:"duration"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, err
	}

	var tracks []*state.Track
	for _, e := range data.Entries {
		if e.ID == "" {
			continue
		}
		tracks = append(tracks, &state.Track{
			URL:      "https://www.youtube.com/watch?v=" + e.ID,
			Title:    e.Title,
			ID:       e.ID,
			Artwork:  "https://i.ytimg.com/vi/" + e.ID + "/hqdefault.jpg",
			Duration: int(e.Duration),
			Source:   PlatformYouTube,
		})
	}
	if len(tracks) == 0 {
		return nil, errors.New("yt-dlp mix is empty")
	}
	return tracks, nil
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

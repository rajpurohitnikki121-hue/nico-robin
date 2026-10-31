package platforms

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"
	"resty.dev/v3"

	state "main/internal/core/models"
)

// ShrutiBots API is used as an extra fallback downloader for YouTube tracks.
//
//	GET {base}/download?url=<youtube url or id>&type=audio&api_key=<key>
//
// The response body is the audio file itself. Configure it with:
//
//	SHRUTI_API_KEY  your key (required, the platform is off without it)
//	SHRUTI_API_URL  optional, default https://api.shrutibots.in
const (
	PlatformShrutiApi state.PlatformName = "ShrutiApi"

	defaultShrutiAPIURL = "https://api.shrutibots.in"
	shrutiMinFileSize   = 4 * 1024
)

type ShrutiApiPlatform struct {
	name state.PlatformName
}

// The API converts the video before answering, so give it more time than the
// shared 20 second client.
var shrutiClient = resty.New().SetTimeout(3 * time.Minute)

func init() {
	// Above FallenApi (80): tried first; FallenApi stays as the fallback.
	Register(90, &ShrutiApiPlatform{name: PlatformShrutiApi})
}

func shrutiConfig() (base, key string) {
	key = strings.TrimSpace(os.Getenv("SHRUTI_API_KEY"))
	base = strings.TrimRight(strings.TrimSpace(os.Getenv("SHRUTI_API_URL")), "/")
	if base == "" {
		base = defaultShrutiAPIURL
	}
	return base, key
}

func (s *ShrutiApiPlatform) Name() state.PlatformName { return s.name }

func (s *ShrutiApiPlatform) CanGetTracks(string) bool { return false }

func (s *ShrutiApiPlatform) GetTracks(string, bool) ([]*state.Track, error) {
	return nil, errors.New("shrutiapi is a download-only platform")
}

func (s *ShrutiApiPlatform) CanDownload(source state.PlatformName) bool {
	_, key := shrutiConfig()
	return key != "" && source == PlatformYouTube
}

func (s *ShrutiApiPlatform) Download(
	ctx context.Context,
	track *state.Track,
	_ *telegram.NewMessage,
) (string, error) {
	// Audio only; let the next downloader handle video requests.
	if track.Video {
		return "", errors.New("shrutiapi: video is not supported")
	}

	if f := findFile(track); f != "" {
		gologging.Debug("ShrutiApi: Download -> Cached File -> " + f)
		return f, nil
	}

	base, key := shrutiConfig()
	if key == "" {
		return "", errors.New("shrutiapi: SHRUTI_API_KEY is not set")
	}
	if strings.TrimSpace(track.URL) == "" {
		return "", errors.New("shrutiapi: empty track url")
	}

	// The key travels in the query string, and request errors contain the
	// URL, so it is always removed from anything that is logged or shown.
	redact := func(msg string) string {
		return strings.ReplaceAll(msg, key, "***")
	}

	resp, err := shrutiClient.R().
		SetContext(ctx).
		SetQueryParam("url", track.URL).
		SetQueryParam("type", "audio").
		SetQueryParam("api_key", key).
		Get(base + "/download")
	if err != nil {
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		return "", fmt.Errorf("shrutiapi request failed: %s", redact(err.Error()))
	}

	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return "", fmt.Errorf(
			"shrutiapi failed with status %d: %s",
			resp.StatusCode(),
			redact(fallenSnippet(resp.String())),
		)
	}

	data := resp.Bytes()
	contentType := strings.ToLower(resp.Header().Get("Content-Type"))
	if strings.HasPrefix(contentType, "application/json") ||
		strings.HasPrefix(contentType, "text/") {
		return "", fmt.Errorf(
			"shrutiapi returned %s instead of audio: %s",
			contentType,
			redact(fallenSnippet(string(data))),
		)
	}
	if len(data) < shrutiMinFileSize {
		return "", fmt.Errorf(
			"shrutiapi returned a too small file (%d bytes)",
			len(data),
		)
	}

	path := getPath(track, ".m4a")
	if err := os.MkdirAll("downloads", 0o755); err != nil {
		return "", fmt.Errorf("shrutiapi: cannot create downloads dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("shrutiapi: failed to save file: %w", err)
	}

	return path, nil
}

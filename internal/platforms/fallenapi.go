/*
 * ● YukkiMusic
 * ○ A high-performance engine for streaming music in Telegram voicechats.
 *
 * Copyright (C) 2026 TheTeamVivek
 *
 * This program is free software: you can redistribute it and/or modify it under the
 * terms of the GNU General Public License as published by the Free Software Foundation,
 * either version 3 of the License, or (at your option) any later version.
 */

package platforms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	"main/internal/core"
	state "main/internal/core/models"
	"main/internal/utils"
)

var telegramDLRegex = regexp.MustCompile(
	`https:\/\/t\.me\/([a-zA-Z0-9_]{5,})\/(\d+)`,
)

const PlatformFallenApi state.PlatformName = "FallenApi"

type FallenApiPlatform struct {
	name state.PlatformName
}

func init() {
	Register(80, &FallenApiPlatform{
		name: PlatformFallenApi,
	})
}

func (f *FallenApiPlatform) Name() state.PlatformName {
	return f.name
}

func (f *FallenApiPlatform) CanGetTracks(query string) bool {
	return false
}

func (f *FallenApiPlatform) GetTracks(
	_ string,
	_ bool,
) ([]*state.Track, error) {
	return nil, errors.New("fallenapi is a download-only platform")
}

func (f *FallenApiPlatform) CanDownload(
	source state.PlatformName,
) bool {
	if config.FallenAPIURL == "" || config.FallenAPIKey == "" {
		return false
	}

	return source == PlatformYouTube
}

func (f *FallenApiPlatform) Download(
	ctx context.Context,
	track *state.Track,
	statusMsg *telegram.NewMessage,
) (string, error) {

	// Audio only.
	track.Video = false

	if f := findFile(track); f != "" {
		gologging.Debug("FallenApi: Download -> Cached File -> " + f)
		return f, nil
	}

	var pm *telegram.ProgressManager
	if statusMsg != nil {
		pm = utils.GetProgress(statusMsg)
	}

	dlURL, err := f.getDownloadURL(ctx, track.URL)
	if err != nil {
		return "", err
	}

	path := getPath(track, ".mp3")

	var downloadErr error

	if telegramDLRegex.MatchString(dlURL) {
		path, downloadErr = f.downloadFromTelegram(ctx, dlURL, path, pm)
	} else {
		downloadErr = f.downloadFromURL(ctx, dlURL, path)
	}

	if downloadErr != nil {
		return "", downloadErr
	}

	if !fileExists(path) {
		return "", errors.New("empty file returned by API")
	}

	return path, nil
}

// oneGrabTrackResponse mirrors the JSON returned by OneGrab's
// GET /api/track endpoint.
type oneGrabTrackResponse struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	CDNURL   string `json:"cdnurl"`
	Platform string `json:"platform"`
}

// fallenSnippet shortens an API response body for use in error messages.
func fallenSnippet(body string) string {
	body = strings.TrimSpace(body)
	if len(body) > 200 {
		return body[:200] + "..."
	}
	return body
}

// getDownloadURL asks the OneGrab API for a CDN URL for the given
// YouTube track.
//
//	GET {apiBase}/api/track?url=<youtube_url>&video=false
//	Header: X-API-Key: <key>
//
// The JSON response contains "cdnurl", which is the URL the audio is
// downloaded from.
func (f *FallenApiPlatform) getDownloadURL(
	ctx context.Context,
	mediaURL string,
) (string, error) {

	if strings.TrimSpace(mediaURL) == "" {
		return "", errors.New("fallenapi: empty track url")
	}

	apiBase := strings.TrimRight(strings.TrimSpace(config.FallenAPIURL), "/")
	if apiBase == "" {
		return "", errors.New("fallenapi: API URL is not configured")
	}

	if videoID, err := extractYouTubeVideoID(mediaURL); err == nil {
		gologging.Debug("FallenApi: requesting track for YouTube video " + videoID)
	}

	resp, err := rc.R().
		SetContext(ctx).
		SetQueryParam("url", mediaURL).
		SetQueryParam("video", "false").
		SetHeader("X-API-Key", config.FallenAPIKey).
		Get(apiBase + "/api/track")
	if err != nil {
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		return "", fmt.Errorf("onegrab /api/track request failed: %w", err)
	}

	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return "", fmt.Errorf(
			"onegrab /api/track failed with status: %d body: %s",
			resp.StatusCode(),
			fallenSnippet(resp.String()),
		)
	}

	body := resp.Bytes()
	if len(body) == 0 {
		return "", errors.New("onegrab /api/track returned an empty response")
	}

	var result oneGrabTrackResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf(
			"onegrab /api/track returned invalid json: %w",
			err,
		)
	}

	cdnURL := strings.TrimSpace(result.CDNURL)
	if cdnURL == "" {
		return "", errors.New("onegrab /api/track response missing cdnurl")
	}

	parsed, err := url.Parse(cdnURL)
	if err != nil {
		return "", fmt.Errorf("onegrab returned invalid cdnurl: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf(
			"onegrab returned unsupported cdnurl: %s",
			fallenSnippet(cdnURL),
		)
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	return cdnURL, nil
}

// extractYouTubeVideoID extracts the video ID from common YouTube URLs.
func extractYouTubeVideoID(mediaURL string) (string, error) {

	parsed, err := url.Parse(mediaURL)
	if err != nil {
		return "", fmt.Errorf("invalid YouTube URL: %w", err)
	}

	host := strings.ToLower(parsed.Hostname())

	// youtube.com/watch?v=VIDEO_ID
	if host == "youtube.com" ||
		host == "www.youtube.com" ||
		host == "m.youtube.com" {

		if videoID := parsed.Query().Get("v"); videoID != "" {
			return videoID, nil
		}

		// youtube.com/shorts/VIDEO_ID
		// youtube.com/embed/VIDEO_ID
		pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")

		if len(pathParts) >= 2 &&
			(pathParts[0] == "shorts" || pathParts[0] == "embed") {
			return pathParts[1], nil
		}
	}

	// youtu.be/VIDEO_ID
	if host == "youtu.be" {
		videoID := strings.Trim(parsed.Path, "/")

		if videoID != "" {
			return videoID, nil
		}
	}

	return "", fmt.Errorf(
		"could not extract YouTube video ID from URL: %s",
		mediaURL,
	)
}

func (f *FallenApiPlatform) downloadFromURL(
	ctx context.Context,
	dlURL, path string,
) error {

	req := rc.R().SetContext(ctx)

	// Only send the API key when the CDN is on the same host as the
	// configured API. It must never be sent to any other host.
	if apiURL, err := url.Parse(strings.TrimSpace(config.FallenAPIURL)); err == nil {
		if cdn, err := url.Parse(dlURL); err == nil &&
			apiURL.Host != "" &&
			strings.EqualFold(cdn.Host, apiURL.Host) {
			req = req.SetHeader("X-API-Key", config.FallenAPIKey)
		}
	}

	resp, err := req.Get(dlURL)
	if err != nil {
		_ = os.Remove(path)

		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		return fmt.Errorf("http download failed: %w", err)
	}

	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		_ = os.Remove(path)
		return fmt.Errorf(
			"download failed with status: %d body: %s",
			resp.StatusCode(),
			fallenSnippet(resp.String()),
		)
	}

	data := resp.Bytes()
	if len(data) == 0 {
		_ = os.Remove(path)
		return errors.New("download returned an empty file")
	}

	// A JSON or HTML body here is an error page, not audio.
	contentType := strings.ToLower(resp.Header().Get("Content-Type"))
	if strings.HasPrefix(contentType, "application/json") ||
		strings.HasPrefix(contentType, "text/html") {
		_ = os.Remove(path)
		return fmt.Errorf(
			"download returned %s instead of audio: %s",
			contentType,
			fallenSnippet(string(data)),
		)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("failed to save downloaded file: %w", err)
	}

	return nil
}

func (f *FallenApiPlatform) downloadFromTelegram(
	ctx context.Context,
	dlURL, path string,
	pm *telegram.ProgressManager,
) (string, error) {
	matches := telegramDLRegex.FindStringSubmatch(dlURL)

	if len(matches) < 3 {
		return "", fmt.Errorf(
			"invalid telegram download url: %s",
			dlURL,
		)
	}

	username := matches[1]

	messageID, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", fmt.Errorf(
			"invalid message ID: %v",
			err,
		)
	}

	msg, err := core.Bot.GetMessageByID(
		username,
		int32(messageID),
	)
	if err != nil {
		return "", fmt.Errorf(
			"failed to fetch Telegram message: %w",
			err,
		)
	}

	dOpts := &telegram.DownloadOptions{
		FileName: path,
		Ctx:      ctx,
	}

	if pm != nil {
		dOpts.ProgressManager = pm
	}

	_, err = msg.Download(dOpts)
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}

	return path, nil
}

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

	// Meow API audio endpoint
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

// yukiDownloadResponse mirrors the JSON body returned by Meow/Yuki
// API's GET /download endpoint: {"status":"success","video_id":"...","download_token":"..."}
type yukiDownloadResponse struct {
	VideoID       string `json:"video_id"`
	DownloadToken string `json:"download_token"`
}

// getDownloadURL resolves a Meow/Yuki API stream URL.
//
// Meow/Yuki API is a TWO-STEP flow, not a single direct-key stream URL:
//  1. GET {apiBase}/download?url=<youtube_url>&type=audio
//     -> JSON {"video_id":"...","download_token":"..."}
//  2. GET {apiBase}/stream/{video_id}?token=<download_token>&type=audio
//     -> the actual audio bytes
//
// NOTE: our own extracted videoID is used to build the stream URL
// rather than the "video_id" the API echoes back in step 1 — that
// field has been observed to sometimes echo an unrelated previous
// request's ID, which would download the wrong song.
func (f *FallenApiPlatform) getDownloadURL(
	ctx context.Context,
	mediaURL string,
) (string, error) {

	videoID, err := extractYouTubeVideoID(mediaURL)
	if err != nil {
		return "", err
	}

	apiBase := strings.TrimRight(config.FallenAPIURL, "/")

	// Step 1: resolve a download_token.
	resolveResp, err := rc.R().
		SetContext(ctx).
		SetQueryParam("url", mediaURL).
		SetQueryParam("type", "audio").
		SetHeader("X-API-Key", config.FallenAPIKey).
		SetHeader("Authorization", "Bearer "+config.FallenAPIKey).
		Get(apiBase + "/download")

	if err != nil {
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		return "", fmt.Errorf("meow api /download request failed: %w", err)
	}

	if resolveResp.IsStatusFailure() {
		return "", fmt.Errorf(
			"meow api /download failed with status: %d body: %s",
			resolveResp.StatusCode(),
			resolveResp.String(),
		)
	}

	var result yukiDownloadResponse
	if err := json.Unmarshal(resolveResp.Bytes(), &result); err != nil {
		return "", fmt.Errorf("meow api /download returned invalid json: %w", err)
	}

	if result.DownloadToken == "" {
		return "", fmt.Errorf("meow api /download response missing download_token")
	}

	// Step 2: build the stream URL using OUR videoID + their token.
	streamURL := fmt.Sprintf(
		"%s/stream/%s?token=%s&type=audio",
		apiBase,
		url.PathEscape(videoID),
		url.QueryEscape(result.DownloadToken),
	)

	gologging.Debug("Meow API: requesting stream for YouTube video " + videoID)

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	return streamURL, nil
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

	resp, err := rc.R().
		SetContext(ctx).
		SetResponseSaveToFile(true).
		SetResponseSaveFileName(path).
		Get(dlURL)

	if err != nil {
		os.Remove(path)

		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		return fmt.Errorf("http download failed: %w", err)
	}

	if resp.IsStatusFailure() {
		os.Remove(path)
		return fmt.Errorf(
			"download failed with status: %d body: %s",
			resp.StatusCode(),
			resp.String(),
		)
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
		os.Remove(path)
		return "", err
	}

	return path, nil
}

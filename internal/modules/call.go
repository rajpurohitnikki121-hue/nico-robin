/*
 * ● YukkiMusic
 * ○ A high-performance engine for streaming music in Telegram voicechats.
 *
 * Copyright (C) 2026 TheTeamVivek
 *
 * This program is free software: you can redistribute it and/or modify it under the
 * terms of the GNU General Public License as published by the Free Software Foundation,
 * either version 3 of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful, but WITHOUT ANY
 * WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
 * PARTICULAR PURPOSE. See the GNU General Public License for more details.
 *
 * Repository: https://github.com/TheTeamVivek/YukkiMusic
 */

package modules

import (
	"context"
	"time"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	"main/internal/core"
	"main/internal/locales"
	"main/internal/utils"
	"main/ntgcalls"
)

func streamEndHandler(
	chatID int64,
	streamType ntgcalls.StreamType,
	_ ntgcalls.StreamDevice,
) {
	if streamType == ntgcalls.VideoStream {
		gologging.Debug("[onStreamEndHandler] Video stream ended, returning")
		return
	}

	gologging.DebugF("[onStreamEndHandler] Stream ended in chat %d", chatID)
	ass, err := core.Assistants.ForChat(chatID)
	if err != nil {
		gologging.ErrorF("Failed to get Assistant for %d: %v", chatID, err)
		return
	}
	r, ok := core.GetRoom(chatID, ass, false)
	if !ok {
		return
	}
	scheduleOldPlayingMessage(r)

	if ok, v := r.GetData("is_transitioning"); ok {
		if ok, v := v.(bool); ok && v {
			return
		}
	}

	r.SetData("is_transitioning", true)
	defer r.DeleteData("is_transitioning")

	cid := r.ChatID
	r.Parse()

	// Autoplay: when the queue has run out, queue a related song so the normal
	// "play next" flow below continues the music.
	if r.Loop() == 0 {
		autoplayQueueIfEmpty(r)
	}

	if len(r.Queue()) == 0 && r.Loop() == 0 {
		finishQueue(chatID, cid)
		return
	}

	wasLooping := r.Loop() > 0
	t := r.NextTrack()

	var (
		statusMsg *telegram.NewMessage
		filePath  string
	)

	// Download the next song. If it cannot be downloaded (after retries) tell
	// the chat and go on with the following song, so one bad song never drops
	// the rest of the queue.
	for {
		if t == nil {
			core.DeleteRoom(chatID)
			return
		}

		reuse := wasLooping && r.FilePath() != ""
		statusText := F(cid, "stream_downloading_next")
		if reuse {
			statusText = F(cid, "cb_replaying")
		}

		var sendErr error
		statusMsg, sendErr = core.Bot.SendMessage(cid, statusText)
		if sendErr != nil {
			gologging.ErrorF("[call.go] Failed to send msg: %v", sendErr)
		}

		var dlErr error
		if reuse {
			filePath = r.FilePath()
		} else {
			filePath, dlErr = downloadWithRetry(
				context.Background(),
				t,
				statusMsg,
			)
		}
		if dlErr == nil {
			break
		}

		gologging.ErrorF(
			"[onStreamEndHandler] Download failed for %s: %v",
			t.URL,
			dlErr,
		)

		if statusMsg != nil {
			utils.EOR(statusMsg, F(cid, "stream_download_skipped", locales.Arg{
				"title": utils.EscapeHTML(utils.ShortTitle(t.Title, 25)),
				"error": utils.EscapeHTML(dlErr.Error()),
			}))
			cleanScheduler.scheduleAfter(cid, statusMsg.ID, queueFinishedDelay)
		}

		r.SetLoop(0)
		wasLooping = false
		if len(r.Queue()) == 0 {
			finishQueue(chatID, cid)
			return
		}
		t = r.NextTrack()
	}

	playErr := r.Play(t, filePath, true)
	if playErr != nil {
		// One more try: a voice chat hiccup is often gone after a moment.
		time.Sleep(2 * time.Second)
		playErr = r.Play(t, filePath, true)
	}
	if playErr != nil {
		gologging.ErrorF(
			"[onStreamEndHandler] Play failed for %s: %v",
			t.URL,
			playErr,
		)
		utils.EOR(statusMsg, F(cid, "stream_play_fail"))
		core.DeleteRoom(chatID)
		return
	}

	title := utils.ShortTitle(t.Title, 25)
	safeTitle := utils.EscapeHTML(title)

	msgText := F(cid, "stream_now_playing", locales.Arg{
		"url":      t.URL,
		"title":    safeTitle,
		"duration": utils.FormatTime(t.Duration),
		"by":       t.Requester,
	})

	opt := &telegram.SendOptions{
		ParseMode:   "HTML",
		ReplyMarkup: core.GetPlayMarkup(cid, r, false),
	}

	if t.Artwork != "" && shouldShowThumb(chatID) {
		opt.Media = thumbMedia(t)
	}

	statusMsg, _ = utils.EOR(statusMsg, msgText, opt)
	r.SetStatusMsg(statusMsg)
	notifyAutoplayStart(cid, t)
}

// finishQueue ends the session when there is nothing left to play.
func finishQueue(roomID, cid int64) {
	core.DeleteRoom(roomID)
	if fin, err := core.Bot.SendMessage(cid, F(cid, "stream_queue_finished")); err == nil && fin != nil {
		// The notice is only needed briefly; remove it automatically.
		cleanScheduler.scheduleAfter(cid, fin.ID, queueFinishedDelay)
	}
}

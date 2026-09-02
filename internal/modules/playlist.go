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

// NOTE: This is a NEW file. It does not modify play.go, skip.go, or any
// other existing module. It reuses existing helpers (extractPlayQuery,
// getCommand, getEffectiveRoom, F, utils.*) that already exist in this
// package, and reuses platforms.GetTracks for song resolution and
// core.RoomState.Play for queueing — the same functions /play uses — so
// /skip and the existing queue system keep working with playlist songs
// automatically, with no separate logic needed.

package modules

import (
	"errors"
	"fmt"
	"strings"

	tg "github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	state "main/internal/core/models"
	"main/internal/database"
	"main/internal/locales"
	"main/internal/platforms"
	"main/internal/utils"
)

func init() {
	helpTexts["/createplaylist"] = `<i>Create a new personal playlist.</i>

<u>Usage:</u>
<b>/createplaylist [name]</b> — Create a playlist and make it your current playlist

<b>💡 Example:</b>
<code>/createplaylist My Favourites</code>

<b>⚠️ Limits:</b>
• Max 5 playlists per user`

	helpTexts["/addplaylist"] = `<i>Add a song to your current playlist.</i>

<u>Usage:</u>
<b>/addplaylist [song name/URL]</b> — Add a song to your current playlist

<b>💡 Example:</b>
<code>/addplaylist Sanam Re</code>

<b>⚠️ Limits:</b>
• Max 50 songs per playlist
• Create a playlist first using /createplaylist`

	helpTexts["/playlist"] = `<i>Play all songs from one of your saved playlists.</i>

<u>Usage:</u>
<b>/playlist [name]</b> — Queue and play a saved playlist

<b>💡 Example:</b>
<code>/playlist My Favourites</code>`

	helpTexts["/myplaylist"] = `<i>Show all your saved playlists.</i>

<u>Usage:</u>
<b>/myplaylist</b> — List your playlists and current selection`

	helpTexts["/removeplaylist"] = `<i>Remove a song from your current playlist.</i>

<u>Usage:</u>
<b>/removeplaylist [song name]</b> — Remove a song from your current playlist

<b>⚠️ Note:</b>
This only edits the saved playlist — it does not affect the current playback queue.`
}

func createPlaylistHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	name := strings.TrimSpace(extractPlayQuery(m.Text()))
	if name == "" {
		m.Reply(F(chatID, "playlist_name_required"))
		return tg.ErrEndGroup
	}

	err := database.CreatePlaylist(m.SenderID(), name)
	switch {
	case errors.Is(err, database.ErrPlaylistLimit):
		m.Reply(
			F(
				chatID,
				"playlist_limit_reached",
				locales.Arg{"limit": database.MaxPlaylistsPerUser},
			),
		)
	case errors.Is(err, database.ErrPlaylistExists):
		m.Reply(
			F(
				chatID,
				"playlist_already_exists",
				locales.Arg{"name": utils.EscapeHTML(name)},
			),
		)
	case err != nil:
		m.Reply(F(chatID, "generic_error", locales.Arg{"error": err.Error()}))
	default:
		m.Reply(F(chatID, "playlist_created", locales.Arg{"name": utils.EscapeHTML(name)}))
	}
	return tg.ErrEndGroup
}

func addPlaylistHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	query := strings.TrimSpace(extractPlayQuery(m.Text()))
	if query == "" && !m.IsReply() {
		m.Reply(F(chatID, "no_song_query", locales.Arg{"cmd": getCommand(m)}))
		return tg.ErrEndGroup
	}

	if _, err := database.CurrentPlaylist(m.SenderID()); err != nil {
		m.Reply(F(chatID, "playlist_none_selected"))
		return tg.ErrEndGroup
	}

	statusMsg, _ := m.Reply(
		F(chatID, "searching_query", locales.Arg{"query": utils.EscapeHTML(query)}),
	)

	tracks, err := platforms.GetTracks(m, false)
	if err != nil || len(tracks) == 0 {
		utils.EOR(statusMsg, F(chatID, "no_song_found"))
		return tg.ErrEndGroup
	}
	track := tracks[0]

	song := database.PlaylistSong{
		ID:       track.ID,
		Title:    track.Title,
		URL:      track.URL,
		Duration: track.Duration,
		Source:   string(track.Source),
	}

	err = database.AddSongToPlaylist(m.SenderID(), song)
	switch {
	case errors.Is(err, database.ErrSongLimit):
		utils.EOR(
			statusMsg,
			F(
				chatID,
				"playlist_song_limit_reached",
				locales.Arg{"limit": database.MaxSongsPerPlaylist},
			),
		)
	case errors.Is(err, database.ErrPlaylistNotFound),
		errors.Is(err, database.ErrNoCurrentPlaylist):
		utils.EOR(statusMsg, F(chatID, "playlist_none_selected"))
	case err != nil:
		utils.EOR(statusMsg, F(chatID, "generic_error", locales.Arg{"error": err.Error()}))
	default:
		utils.EOR(statusMsg, F(chatID, "playlist_song_added", locales.Arg{
			"title": utils.EscapeHTML(utils.ShortTitle(track.Title, 35)),
		}))
	}
	return tg.ErrEndGroup
}

func playlistHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	name := strings.TrimSpace(extractPlayQuery(m.Text()))
	if name == "" {
		m.Reply(F(chatID, "playlist_name_required"))
		return tg.ErrEndGroup
	}

	pl, err := database.GetPlaylist(m.SenderID(), name)
	if err != nil {
		m.Reply(F(chatID, "playlist_not_found", locales.Arg{"name": utils.EscapeHTML(name)}))
		return tg.ErrEndGroup
	}
	if len(pl.Songs) == 0 {
		m.Reply(F(chatID, "playlist_empty", locales.Arg{"name": utils.EscapeHTML(name)}))
		return tg.ErrEndGroup
	}

	room, err := getEffectiveRoom(m, false)
	if err != nil {
		m.Reply(err.Error())
		return tg.ErrEndGroup
	}
	room.Parse()

	if len(room.Queue()) >= config.QueueLimit {
		m.Reply(F(chatID, "queue_limit_reached", locales.Arg{"limit": config.QueueLimit}))
		return tg.ErrEndGroup
	}

	mention := utils.MentionHTML(m.Sender)
	added := 0

	// Songs are played in the exact order they were saved. Each call reuses
	// the SAME room.Play() the /play command uses, so tracks land in the
	// existing queue — no separate playlist queue/player is created, and
	// /skip continues to work without any playlist-specific logic.
	for _, s := range pl.Songs {
		if len(room.Queue()) >= config.QueueLimit {
			break
		}
		track := &state.Track{
			ID:        s.ID,
			Title:     s.Title,
			URL:       s.URL,
			Duration:  s.Duration,
			Source:    state.PlatformName(s.Source),
			Requester: mention,
		}
		if err := room.Play(track, "", false); err != nil {
			continue
		}
		added++
	}

	if added == 0 {
		m.Reply(F(chatID, "playlist_play_failed"))
		return tg.ErrEndGroup
	}

	m.Reply(F(chatID, "playlist_playing", locales.Arg{
		"name":  utils.EscapeHTML(name),
		"count": added,
		"by":    mention,
	}))
	return tg.ErrEndGroup
}

func myPlaylistHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()

	playlists, err := database.UserPlaylists(m.SenderID())
	if err != nil {
		m.Reply(F(chatID, "generic_error", locales.Arg{"error": err.Error()}))
		return tg.ErrEndGroup
	}
	if len(playlists) == 0 {
		m.Reply(F(chatID, "playlist_none_yet"))
		return tg.ErrEndGroup
	}

	current, _ := database.CurrentPlaylist(m.SenderID())

	var b strings.Builder
	b.WriteString(F(chatID, "playlist_list_header"))
	b.WriteString("\n\n")
	for _, pl := range playlists {
		b.WriteString(
			fmt.Sprintf("♡ %s — %d songs\n", utils.EscapeHTML(pl.Name), len(pl.Songs)),
		)
	}
	if current != "" {
		b.WriteString("\n")
		b.WriteString(
			F(chatID, "playlist_current_line", locales.Arg{"name": utils.EscapeHTML(current)}),
		)
	}

	m.Reply(b.String())
	return tg.ErrEndGroup
}

func removePlaylistHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	title := strings.TrimSpace(extractPlayQuery(m.Text()))
	if title == "" {
		m.Reply(F(chatID, "playlist_song_name_required"))
		return tg.ErrEndGroup
	}

	err := database.RemoveSongFromPlaylist(m.SenderID(), title)
	switch {
	case errors.Is(err, database.ErrNoCurrentPlaylist), errors.Is(err, database.ErrPlaylistNotFound):
		m.Reply(F(chatID, "playlist_none_selected"))
	case errors.Is(err, database.ErrSongNotFound):
		m.Reply(F(chatID, "playlist_song_not_found", locales.Arg{"title": utils.EscapeHTML(title)}))
	case err != nil:
		m.Reply(F(chatID, "generic_error", locales.Arg{"error": err.Error()}))
	default:
		m.Reply(F(chatID, "playlist_song_removed", locales.Arg{"title": utils.EscapeHTML(title)}))
	}
	return tg.ErrEndGroup
}

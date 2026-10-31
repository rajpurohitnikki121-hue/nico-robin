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
	"fmt"
	"html"
	"strings"

	"github.com/Laky-64/gologging"
	tg "github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	"main/internal/core"
)

var helpTexts = map[string]string{}

func init() {
	helpTexts["/help"] = fmt.Sprintf(`ℹ️ <b>Help Command</b>
<i>Displays general bot help or detailed information about a specific command.</i>

<u>Usage:</u>
<code>/help</code> — Show the main help menu.
<code>/help &lt;command&gt;</code> — Show help for a specific command.

For more info, visit our <a href="%s">Support Chat</a>.`, config.SupportChat)
}

// boldFont converts ASCII letters/digits to the mathematical bold font used on
// the bot's buttons (same style as the Owner button).
func boldFont(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(0x1D400 + (r - 'A'))
		case r >= 'a' && r <= 'z':
			sb.WriteRune(0x1D41A + (r - 'a'))
		case r >= '0' && r <= '9':
			sb.WriteRune(0x1D7CE + (r - '0'))
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// helpLine is one row of a help category. cmds are shown together, e.g.
// "/pause, /cpause" (group command and its channel variant).
type helpLine struct {
	cmds string
	desc string
}

type helpGroup struct {
	id    string
	label string
	about string
	lines []helpLine
}

const helpChannelNote = "Commands starting with c work in the linked channel's voice chat. Set it up first with /setcplay."

var helpGroups = []helpGroup{
	{
		id: "play", label: "Play",
		about: "Play songs and videos in the voice chat. " + helpChannelNote,
		lines: []helpLine{
			{"/play, /cplay", "Play a song or link."},
			{"/fplay, /cfplay", "Force play, skips the queue."},
			{"/vplay, /vcplay", "Play a video."},
			{"/fvplay, /fvcplay", "Force play a video."},
			{"/setcplay", "Link a channel for channel play."},
		},
	},
	{
		id: "auto", label: "Autoplay",
		about: "Keep the music going. When the queue ends I play a related song by myself. You can also tap the Auto button on the player.",
		lines: []helpLine{
			{"/autoplay", "Turn autoplay on or off."},
			{"/autoplay enable", "Turn autoplay on."},
			{"/autoplay disable", "Turn autoplay off."},
		},
	},
	{
		id: "ctrl", label: "Controls",
		about: "Control the song that is playing. " + helpChannelNote,
		lines: []helpLine{
			{"/pause, /cpause", "Pause the song."},
			{"/resume, /cresume", "Resume the song."},
			{"/skip, /cskip", "Skip to the next song."},
			{"/stop, /cstop", "Stop and leave the voice chat."},
			{"/mute, /cmute", "Mute the bot."},
			{"/unmute, /cunmute", "Unmute the bot."},
			{"/seek, /cseek", "Seek forward."},
			{"/seekback, /cseekback", "Seek backward."},
			{"/replay, /creplay", "Restart the current song."},
			{"/speed, /cspeed", "Change the playback speed."},
			{"/loop, /cloop", "Loop the current song."},
			{"/shuffle, /cshuffle", "Shuffle the queue."},
		},
	},
	{
		id: "queue", label: "Queue",
		about: "See and manage the queue. " + helpChannelNote,
		lines: []helpLine{
			{"/queue, /cqueue", "Show the queue."},
			{"/position, /cposition", "Current song position."},
			{"/jump, /cjump", "Jump to a song."},
			{"/remove, /cremove", "Remove a song."},
			{"/move, /cmove", "Move a song."},
			{"/clear, /cclear", "Clear the queue."},
			{"/restore, /crestore", "Restore the cleared queue."},
		},
	},
	{
		id: "plist", label: "Playlist",
		about: "Create and play your own personal playlists.",
		lines: []helpLine{
			{"/createplaylist", "Create a playlist."},
			{"/addplaylist", "Add the current song."},
			{"/removeplaylist", "Remove a song."},
			{"/myplaylist", "Show your playlists."},
			{"/playlist", "Play a saved playlist."},
		},
	},
	{
		id: "admin", label: "Admin",
		about: "Group settings and who can use the bot. " + helpChannelNote,
		lines: []helpLine{
			{"/addauth, /delauth", "Add or remove an authorized user."},
			{"/authlist", "List authorized users."},
			{"/adminmode", "Who can use admin commands."},
			{"/playmode", "Who can use /play."},
			{"/settings", "Chat settings."},
			{"/nothumb", "Thumbnails on or off."},
			{"/reload, /creload", "Refresh the admin list."},
		},
	},
	{
		id: "bcast", label: "Broadcast",
		about: "Send a message to every chat and user of the bot. Owner only.",
		lines: []helpLine{
			{"/broadcast [text]", "Broadcast a text message."},
			{"/broadcast (reply)", "Broadcast the message you reply to."},
			{"--nochat", "Skip groups."},
			{"--nouser", "Skip users."},
			{"--copy", "Send without the forwarded tag."},
			{"--pin, --pinloud", "Pin the message (silent or with sound)."},
			{"--limit [n]", "Stop after n messages."},
			{"--delay [sec]", "Wait between messages."},
			{"/broadcast -cancel", "Stop a running broadcast."},
		},
	},
	{
		id: "thumb", label: "Thumbnail",
		about: "Show or hide the song artwork on the player and now-playing messages.",
		lines: []helpLine{
			{"/nothumb", "Show the current setting."},
			{"/nothumb enable", "Hide the artwork."},
			{"/nothumb disable", "Show the artwork again."},
			{"Thumb button", "Tap Thumb ON/OFF on the player to switch."},
		},
	},
	{
		id: "stream", label: "Stream",
		about: "Stream an RTMP source in the voice chat.",
		lines: []helpLine{
			{"/setrtmp", "Set the RTMP URL (in DM)."},
			{"/stream", "Start the RTMP stream."},
			{"/streamstatus", "Stream status."},
			{"/streamstop", "Stop the stream."},
		},
	},
	{
		id: "gen", label: "General",
		about: "Everyday commands for everyone.",
		lines: []helpLine{
			{"/start", "Open the main menu."},
			{"/help", "Open this help menu."},
			{"/ping", "Check if the bot is alive."},
			{"/lang", "Change the language."},
			{"/bug", "Report a bug."},
			{"/privacy", "Privacy policy."},
		},
	},
	{
		id: "sudo", label: "Sudo",
		about: "For sudo users and the owner only.",
		lines: []helpLine{
			{"/ac", "Active voice chats."},
			{"/stats", "Bot statistics."},
			{"/speedtest", "Server speed test."},
			{"/broadcast", "Message all chats."},
			{"/autoleave", "Auto leave on or off."},
			{"/logger", "Logger on or off."},
			{"/logs", "Get the log file."},
			{"/sudoers", "List sudo users."},
			{"/addsudo, /delsudo", "Add or remove a sudo user."},
			{"/blockuser, /unblockuser", "Block or unblock a user."},
			{"/blockchat, /unblockchat", "Block or unblock a chat."},
			{"/blacklisted", "List blocked users and chats."},
			{"/maintenance", "Maintenance mode."},
			{"/restart", "Restart the bot."},
		},
	},
}

func helpGroupByID(id string) *helpGroup {
	for i := range helpGroups {
		if helpGroups[i].id == id {
			return &helpGroups[i]
		}
	}
	return nil
}

// helpMainMarkup is the category grid: three buttons per row (the last one is
// Language), then Back.
func helpMainMarkup(chatID int64) *tg.ReplyInlineMarkup {
	// Blue and green buttons; the colours swap every few seconds.
	tint := core.NewTinter()

	btns := make([]tg.KeyboardButton, 0, len(helpGroups)+1)
	for _, g := range helpGroups {
		btns = append(btns, tint(tg.Button.Data(boldFont(g.label), "help:g:"+g.id)))
	}
	btns = append(btns, tint(tg.Button.Data(boldFont("Language"), "lang:help")))

	kb := tg.NewKeyboard()
	kb.NewColumn(helpCols, btns...)
	kb.AddRow(tint(tg.Button.Data(F(chatID, "BACK_BTN"), "start")))
	return kb.Build()
}

// helpGroupText builds the page of one category.
func helpGroupText(g *helpGroup) string {
	var sb strings.Builder
	sb.WriteString("<u>" + html.EscapeString(g.label) + " Commands:</u>\n\n")
	sb.WriteString("<i>" + html.EscapeString(g.about) + "</i>\n\n")
	for _, l := range g.lines {
		sb.WriteString("• " + html.EscapeString(l.cmds) + " : " +
			html.EscapeString(l.desc) + "\n")
	}
	sb.WriteString("\n<i>Full usage of a command:</i> /help play")
	return sb.String()
}

// helpEdit edits the help message. A photo caption is limited to 1024
// characters, so when the edit is rejected the message is replaced by a new
// text message instead of leaving the user without the help text.
func helpEdit(c *tg.CallbackQuery, text string, opts *tg.SendOptions) {
	_, err := c.Edit(text, opts)
	if err == nil || tg.MatchError(err, "MESSAGE_NOT_MODIFIED") {
		return
	}

	gologging.DebugF("help: edit failed, sending a new message: %v", err)
	if _, respErr := c.Respond(text, opts); respErr != nil {
		gologging.ErrorF("help: send failed: %v", respErr)
		return
	}
	_, _ = c.Delete()
}

func showHelpMain(c *tg.CallbackQuery) error {
	chatID := c.ChannelID()
	helpEdit(
		c,
		F(chatID, "help_main"),
		&tg.SendOptions{ReplyMarkup: helpMainMarkup(chatID)},
	)
	return tg.ErrEndGroup
}

const helpCols = 3

func showHelpGroup(c *tg.CallbackQuery, id string) error {
	g := helpGroupByID(id)
	if g == nil {
		return showHelpMain(c)
	}
	chatID := c.ChannelID()
	tint := core.NewTinter()
	back := tg.NewKeyboard().AddRow(
		tint(tg.Button.Data(F(chatID, "BACK_BTN"), "help:main")),
		tint(tg.Button.Data(boldFont("Close"), "close")),
	).Build()
	helpEdit(c, helpGroupText(g), &tg.SendOptions{ReplyMarkup: back})
	return tg.ErrEndGroup
}

func helpHandler(m *tg.NewMessage) error {
	args := strings.Fields(m.Text())
	if len(args) > 1 {
		cmd := args[1]
		if cmd != "pm_help" {
			if !strings.HasPrefix(cmd, "/") {
				cmd = "/" + cmd
			}
			return showHelpFor(m, cmd)
		}
	}

	if m.ChatType() != tg.EntityUser {
		m.Reply(
			F(m.ChannelID(), "help_private_only"),
			&tg.SendOptions{
				ReplyMarkup: core.GetGroupHelpKeyboard(m.ChannelID()),
			},
		)
		return tg.ErrEndGroup
	}

	m.Reply(
		F(m.ChannelID(), "help_main"),
		&tg.SendOptions{ReplyMarkup: helpMainMarkup(m.ChannelID())},
	)
	return tg.ErrEndGroup
}

func helpCB(c *tg.CallbackQuery) error {
	c.Answer("")
	return showHelpMain(c)
}

func helpCallbackHandler(c *tg.CallbackQuery) error {
	data := c.DataString()
	c.Answer("")
	if data == "" {
		return tg.ErrEndGroup
	}
	chatID := c.ChannelID()
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return tg.ErrEndGroup
	}

	// Categories: "help:g:<id>". Old paged-grid buttons ("help:pg:", "help:c:")
	// on messages sent earlier simply open the main menu.
	switch {
	case strings.HasPrefix(parts[1], "g:"):
		return showHelpGroup(c, strings.TrimPrefix(parts[1], "g:"))
	case strings.HasPrefix(parts[1], "pg:"), strings.HasPrefix(parts[1], "c:"):
		return showHelpMain(c)
	}

	var text string
	btn := core.GetBackKeyboard(chatID)

	switch parts[1] {
	case "admins":
		text = F(chatID, "help_admin")
	case "sudoers":
		text = F(chatID, "help_sudo")
	case "owner":
		text = F(chatID, "help_owner")
	case "public":
		text = F(chatID, "help_public")
	case "main":
		return showHelpMain(c)
	}

	c.Edit(text, &tg.SendOptions{ReplyMarkup: btn})
	return tg.ErrEndGroup
}

func showHelpFor(m *tg.NewMessage, cmd string) error {
	helpText, ok := helpTexts[cmd]
	if !ok {
		trimmed := strings.TrimPrefix(cmd, "/")
		if value, exists := helpTexts[trimmed]; exists {
			helpText = value
		}
	}

	if helpText == "" {
		_, err := m.Reply(
			"⚠️ <i>No help found for command <code>" +
				html.EscapeString(cmd) +
				"</code></i>",
		)
		if err != nil {
			return err
		}
		return tg.ErrEndGroup
	}

	_, err := m.Reply(
		"📘 <b>Help for</b> <code>" + cmd + "</code>:\n\n" + helpText,
	)
	if err != nil {
		return err
	}
	return tg.ErrEndGroup
}

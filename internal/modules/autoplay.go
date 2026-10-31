package modules

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Laky-64/gologging"
	tg "github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	"main/internal/core"
	state "main/internal/core/models"
	"main/internal/database"
	"main/internal/locales"
	"main/internal/platforms"
	"main/internal/utils"
)

const autoplayRequester = "ᴀᴜᴛᴏᴘʟᴀʏ"

func init() {
	// Tell the player keyboard (package core) the current switches.
	core.AutoplayOn = func(chatID int64) bool {
		on, _ := database.AutoPlayEnabled(chatID)
		return on
	}
	// Every new voice chat session starts with autoplay OFF.
	core.OnSessionStart = func(chatID int64) {
		if on, err := database.AutoPlayEnabled(chatID); err == nil && on {
			_ = database.SetAutoPlay(chatID, false)
		}
	}
	core.ThumbOn = func(chatID int64) bool {
		off, err := database.ThumbnailsDisabled(chatID)
		return err != nil || !off
	}

	helpTexts["/autoplay"] = `<i>Keep the music going: when the queue ends, play a related song automatically.</i>

<u>Usage:</u>
<b>/autoplay</b> — Turn autoplay on or off
<b>/autoplay [enable|disable]</b> — Set it directly

<b>💡 Note:</b>
You can also tap the <b>Auto</b> button on the player. Autoplay picks songs related to the last one played and never repeats a song in the same session. Use /stop to end the session.`
}

// autoplayHandler turns autoplay on or off for the chat.
func autoplayHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()

	current, err := database.AutoPlayEnabled(chatID)
	if err != nil {
		m.Reply(F(chatID, "autoplay_fetch_fail"))
		return tg.ErrEndGroup
	}

	// No argument: flip the current state.
	value := !current
	if args := m.Args(); args != "" {
		value, err = utils.ParseBool(args)
		if err != nil {
			m.Reply(F(chatID, "invalid_bool"))
			return tg.ErrEndGroup
		}
	}

	msgKey := utils.IfElse(value, "autoplay_on_cb", "autoplay_off_cb")

	if value != current {
		if err := database.SetAutoPlay(chatID, value); err != nil {
			m.Reply(F(chatID, "autoplay_update_fail"))
			return tg.ErrEndGroup
		}
	}

	m.Reply(F(chatID, msgKey))
	if r, ok := core.GetRoom(chatID, nil, false); ok {
		refreshPlayMarkup(r)
	}
	return tg.ErrEndGroup
}

// handleAutoplayAction is the "Auto" button on the player card: it flips
// autoplay for the chat and shows the new state.
func handleAutoplayAction(cb *tg.CallbackQuery, r *core.RoomState) error {
	opt := &tg.CallbackOptions{Alert: true}
	chatID := cb.ChannelID()

	current, err := database.AutoPlayEnabled(chatID)
	if err != nil {
		cb.Answer(F(chatID, "autoplay_fetch_fail"), opt)
		return tg.ErrEndGroup
	}

	if err := database.SetAutoPlay(chatID, !current); err != nil {
		cb.Answer(F(chatID, "autoplay_update_fail"), opt)
		return tg.ErrEndGroup
	}

	cb.Answer(F(chatID, utils.IfElse(!current, "autoplay_on_cb", "autoplay_off_cb")), opt)
	refreshPlayMarkup(r)
	return tg.ErrEndGroup
}

// handleThumbAction is the "Thumb" button on the player card: it turns the
// thumbnails of this chat on or off. Only chat admins (and the bot owner or
// sudo users) may use it; autoplay stays open to admins and authorised users.
func handleThumbAction(cb *tg.CallbackQuery, r *core.RoomState) error {
	opt := &tg.CallbackOptions{Alert: true}
	chatID := cb.ChannelID()

	if !isOwnerOrSudo(cb.SenderID) {
		isAdmin, err := utils.IsChatAdmin(cb.Client, chatID, cb.SenderID)
		if err != nil || !isAdmin {
			cb.Answer(F(chatID, "thumb_admin_only_cb"), opt)
			return tg.ErrEndGroup
		}
	}

	disabled, err := database.ThumbnailsDisabled(chatID)
	if err != nil {
		cb.Answer(F(chatID, "nothumb_fetch_fail"), opt)
		return tg.ErrEndGroup
	}

	if err := database.SetThumbnailsDisabled(chatID, !disabled); err != nil {
		cb.Answer(F(chatID, "nothumb_update_fail"), opt)
		return tg.ErrEndGroup
	}

	// It was hidden before (disabled), so it is ON now.
	cb.Answer(F(chatID, utils.IfElse(disabled, "thumb_on_cb", "thumb_off_cb")), opt)
	refreshPlayMarkup(r)
	return tg.ErrEndGroup
}

// refreshPlayMarkup redraws the player buttons right away, so the ON/OFF
// labels change without waiting for the next monitor tick (which also skips
// paused rooms).
func refreshPlayMarkup(r *core.RoomState) {
	if r == nil {
		return
	}
	statusMsg := r.StatusMsg()
	if statusMsg == nil {
		return
	}
	r.Parse()
	statusMsg.Edit(statusMsg.Text(), &tg.SendOptions{
		ReplyMarkup: core.GetPlayMarkup(r.ChatID, r, false),
		Entities:    statusMsg.Message.Entities,
	})
}

// autoplayQueueIfEmpty adds an autoplay track when nothing is queued after the
// current one. It is used when a song ends and when someone skips the last
// song, so the music keeps going while autoplay is on.
func autoplayQueueIfEmpty(r *core.RoomState) {
	if len(r.Queue()) != 0 {
		return
	}
	if next := autoplayNext(r); next != nil {
		r.AddTracksToQueue([]*state.Track{next})
	}
}

// ---- autoplay play logs (logger group) ----

type chatLogInfo struct {
	name string
	link string
}

var chatLogInfos sync.Map // chat id -> chatLogInfo

// rememberChatLogInfo keeps the group name/link of a chat from a command
// message, so logs of songs that play without a command (autoplay) can name
// the group.
func rememberChatLogInfo(m *tg.NewMessage) {
	if m == nil || m.Channel == nil {
		return
	}
	info := chatLogInfo{name: m.Channel.Title}
	if m.Channel.Username != "" {
		info.name = "@" + m.Channel.Username
		info.link = m.Link()
	}
	chatLogInfos.Store(m.ChannelID(), info)
}

// notifyAutoplayStart sends the logger-group log when t was picked by autoplay.
func notifyAutoplayStart(cid int64, t *state.Track) {
	if t != nil && t.Requester == autoplayRequester {
		go sendAutoplayLog(cid, t)
	}
}

func sendAutoplayLog(cid int64, track *state.Track) {
	if config.LoggerID == 0 || config.LoggerID == cid || !isLoggerEnabled() {
		return
	}

	group := fmt.Sprintf("%d", cid)
	link := ""
	if v, ok := chatLogInfos.Load(cid); ok {
		if info, ok := v.(chatLogInfo); ok {
			if info.name != "" {
				group = info.name
			}
			link = info.link
		}
	}

	header := F(cid, "logger_playback_autoplay")

	var sb strings.Builder
	sb.WriteString("🎵 ")
	if link != "" {
		fmt.Fprintf(&sb, "<b><a href=\"%s\">%s</a></b>\n\n", link, header)
	} else {
		fmt.Fprintf(&sb, "<b><u>%s</u></b>\n\n", header)
	}
	sb.WriteString(F(cid, "logger_autoplay_template", locales.Arg{
		"track_url": track.URL,
		"track":     utils.EscapeHTML(utils.ShortTitle(track.Title)),
		"source":    string(track.Source),
		"group":     utils.EscapeHTML(group),
		"group_id":  cid,
	}))

	opts := &tg.SendOptions{ParseMode: "HTML", LinkPreview: false}
	if _, err := core.Bot.SendMessage(config.LoggerID, sb.String(), opts); err != nil {
		gologging.Error("failed to send autoplay logger msg: " + err.Error())
	}
}

// autoplayNext returns the track autoplay should play after the current one,
// or nil when autoplay is off or nothing suitable was found. It remembers the
// songs it has seen in the room so the same song is not picked again.
func autoplayNext(r *core.RoomState) *state.Track {
	enabled, err := database.AutoPlayEnabled(r.ChatID)
	if err != nil || !enabled {
		return nil
	}

	cur := r.Track()
	if cur == nil {
		return nil
	}

	seen := map[string]bool{}
	if ok, v := r.GetData("autoplay_seen"); ok {
		if m, ok := v.(map[string]bool); ok {
			seen = m
		}
	}
	seen[cur.ID] = true

	next, err := platforms.RelatedTrack(cur, seen, config.DurationLimit)
	if err != nil || next == nil {
		gologging.WarnF("[autoplay] no related track for %q: %v", cur.Title, err)
		return nil
	}

	seen[next.ID] = true
	r.SetData("autoplay_seen", seen)
	next.Requester = autoplayRequester
	return next
}

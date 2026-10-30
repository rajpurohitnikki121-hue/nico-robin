package modules

import (
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

	action := utils.IfElse(value, "enabled", "disabled")

	if value == current {
		m.Reply(F(chatID, "autoplay_already", locales.Arg{"action": action}))
		return tg.ErrEndGroup
	}

	if err := database.SetAutoPlay(chatID, value); err != nil {
		m.Reply(F(chatID, "autoplay_update_fail"))
		return tg.ErrEndGroup
	}

	m.Reply(F(chatID, "autoplay_updated", locales.Arg{"action": action}))
	return tg.ErrEndGroup
}

// handleAutoplayAction is the "Auto" button on the player card: it flips
// autoplay for the chat and shows the new state.
func handleAutoplayAction(cb *tg.CallbackQuery) error {
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

	action := utils.IfElse(!current, "enabled", "disabled")
	cb.Answer(F(chatID, "autoplay_updated", locales.Arg{"action": action}), opt)
	return tg.ErrEndGroup
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

package modules

import (
	"fmt"
	"time"

	"github.com/Laky-64/gologging"
	tg "github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	"main/internal/core"
)

// Alive check for the logger group. The bot posts one message there when it
// starts and edits it every few minutes. If the time in that message stops
// moving, the bot is dead (or restarting).
const heartbeatEvery = 5 * time.Minute

func heartbeatText(started bool) string {
	ist := time.FixedZone("IST", 5*3600+1800)
	name := "Bot"
	if core.Bot != nil && core.Bot.Me() != nil {
		name = "@" + core.Bot.Me().Username
	}
	head := "🟢 <b>" + name + " is alive</b>"
	if started {
		head = "🚀 <b>" + name + " started</b>"
	}
	return fmt.Sprintf("%s\n\n⏱ Uptime: <code>%s</code>\n🕒 Last check: <code>%s</code>\n\n<i>If this time stops changing, the bot is down.</i>",
		head,
		formatUptime(time.Since(config.StartTime)),
		time.Now().In(ist).Format("02 Jan 2006 • 15:04:05"),
	)
}

// loggerHeartbeat runs in the background for the life of the bot.
func loggerHeartbeat() {
	if config.LoggerID == 0 {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			gologging.ErrorF("heartbeat panic: %v", r)
		}
	}()

	msg, err := core.Bot.SendMessage(config.LoggerID, heartbeatText(true))
	if err != nil {
		gologging.ErrorF("heartbeat: first send failed: %v", err)
	}

	t := time.NewTicker(heartbeatEvery)
	defer t.Stop()
	for range t.C {
		text := heartbeatText(false)
		if msg != nil {
			if _, err := msg.Edit(text, &tg.SendOptions{}); err == nil {
				continue
			}
		}
		if m2, err := core.Bot.SendMessage(config.LoggerID, text); err == nil {
			msg = m2
		} else {
			gologging.ErrorF("heartbeat: send failed: %v", err)
		}
	}
}

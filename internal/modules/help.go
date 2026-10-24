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
	"sort"
	"strconv"
	"strings"
	"sync"

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

const (
	// helpPageSize is the number of command buttons per help page
	// (3 buttons per row, 5 rows).
	helpPageSize = 15
	helpCols     = 3
)

// helpHiddenCmds are developer-only commands that are kept out of the
// public help list.
var helpHiddenCmds = map[string]bool{
	"sh": true, "bash": true, "shell": true,
	"eval": true, "ev": true, "json": true,
}

var (
	helpListOnce sync.Once
	helpList     []string
)

// helpTextOf returns the help text of a command given without the leading "/".
// helpTexts keys are registered both with and without the slash.
func helpTextOf(name string) string {
	if t, ok := helpTexts["/"+name]; ok {
		return t
	}
	return helpTexts[name]
}

// helpCommands returns the commands shown in the help menu: the bot's
// registered commands first (in their menu order), then any other command that
// has a help text. Aliases that share the same help text are shown once.
func helpCommands() []string {
	helpListOnce.Do(func() {
		seenName := map[string]bool{}
		seenText := map[string]bool{}

		add := func(raw string) {
			name := strings.TrimPrefix(strings.ToLower(raw), "/")
			if name == "" || seenName[name] || helpHiddenCmds[name] {
				return
			}
			text := helpTextOf(name)
			if text == "" || seenText[text] {
				return
			}
			seenName[name] = true
			seenText[text] = true
			helpList = append(helpList, name)
		}

		groups := [][]*tg.BotCommand{
			AllCommands.PrivateUserCommands,
			AllCommands.GroupUserCommands,
			AllCommands.GroupAdminCommands,
			AllCommands.PrivateSudoCommands,
			AllCommands.PrivateOwnerCommands,
		}
		for _, g := range groups {
			for _, c := range g {
				add(c.Command)
			}
		}

		// Commands that are not in the bot menu. Several aliases can share one
		// help text (e.g. bcast / gcast / broadcast); the longest name is the
		// most descriptive, so that one is listed.
		best := map[string]string{}
		for k := range helpTexts {
			name := strings.TrimPrefix(strings.ToLower(k), "/")
			text := helpTextOf(name)
			if name == "" || helpHiddenCmds[name] || text == "" || seenText[text] {
				continue
			}
			if cur, ok := best[text]; !ok || len(name) > len(cur) ||
				(len(name) == len(cur) && name < cur) {
				best[text] = name
			}
		}
		rest := make([]string, 0, len(best))
		for _, name := range best {
			rest = append(rest, name)
		}
		sort.Strings(rest)
		for _, name := range rest {
			add(name)
		}
	})
	return helpList
}

// helpPageCount returns how many pages the command list needs.
func helpPageCount() int {
	n := (len(helpCommands()) + helpPageSize - 1) / helpPageSize
	if n < 1 {
		return 1
	}
	return n
}

// clampHelpPage keeps a page number inside the valid range.
func clampHelpPage(page int) int {
	if page < 0 {
		return 0
	}
	if last := helpPageCount() - 1; page > last {
		return last
	}
	return page
}

// helpPageText is the message shown above the command grid.
func helpPageText(chatID int64, page int) string {
	return F(chatID, "help_main") +
		"\n\n<i>Page " + strconv.Itoa(page+1) + "/" +
		strconv.Itoa(helpPageCount()) + "</i>"
}

// helpPageMarkup builds one page of the command grid (3 buttons per row) with
// Prev / Next buttons when there are more commands and a Back button to the
// start menu.
func helpPageMarkup(chatID int64, page int) *tg.ReplyInlineMarkup {
	cmds := helpCommands()
	pages := helpPageCount()
	start := page * helpPageSize
	end := min(start+helpPageSize, len(cmds))

	kb := tg.NewKeyboard()

	for i := start; i < end; i += helpCols {
		stop := min(i+helpCols, end)
		switch stop - i {
		case 3:
			kb.AddRow(
				tg.Button.Data("/"+cmds[i], "help:c:"+cmds[i]),
				tg.Button.Data("/"+cmds[i+1], "help:c:"+cmds[i+1]),
				tg.Button.Data("/"+cmds[i+2], "help:c:"+cmds[i+2]),
			)
		case 2:
			kb.AddRow(
				tg.Button.Data("/"+cmds[i], "help:c:"+cmds[i]),
				tg.Button.Data("/"+cmds[i+1], "help:c:"+cmds[i+1]),
			)
		default:
			kb.AddRow(
				tg.Button.Data("/"+cmds[i], "help:c:"+cmds[i]),
			)
		}
	}

	prev := "help:pg:" + strconv.Itoa(page-1)
	next := "help:pg:" + strconv.Itoa(page+1)
	switch {
	case page > 0 && page < pages-1:
		kb.AddRow(
			tg.Button.Data("‹ Prev", prev),
			tg.Button.Data("Next ›", next),
		)
	case page > 0:
		kb.AddRow(tg.Button.Data("‹ Prev", prev))
	case page < pages-1:
		kb.AddRow(tg.Button.Data("Next ›", next))
	}

	kb.AddRow(tg.Button.Data(F(chatID, "BACK_BTN"), "start"))
	return kb.Build()
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

func showHelpPage(c *tg.CallbackQuery, page int) error {
	chatID := c.ChannelID()
	page = clampHelpPage(page)
	helpEdit(
		c,
		helpPageText(chatID, page),
		&tg.SendOptions{ReplyMarkup: helpPageMarkup(chatID, page)},
	)
	return tg.ErrEndGroup
}

// showHelpCommand shows how to use a single command, with a Back button that
// returns to the page of the command list the command is on.
func showHelpCommand(c *tg.CallbackQuery, name string) error {
	chatID := c.ChannelID()
	name = strings.TrimPrefix(strings.ToLower(name), "/")

	page := 0
	for i, n := range helpCommands() {
		if n == name {
			page = i / helpPageSize
			break
		}
	}

	back := tg.NewKeyboard().
		AddRow(
			tg.Button.Data(F(chatID, "BACK_BTN"), "help:pg:"+strconv.Itoa(page)),
		).
		Build()

	text := helpTextOf(name)
	if text == "" {
		text = "⚠️ <i>No help found for command <code>/" +
			html.EscapeString(name) + "</code></i>"
	} else {
		text = "<b>⦿ /" + html.EscapeString(name) + "</b>\n\n" + text
	}

	helpEdit(c, text, &tg.SendOptions{ReplyMarkup: back})
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
		helpPageText(m.ChannelID(), 0),
		&tg.SendOptions{ReplyMarkup: helpPageMarkup(m.ChannelID(), 0)},
	)
	return tg.ErrEndGroup
}

func helpCB(c *tg.CallbackQuery) error {
	c.Answer("")
	return showHelpPage(c, 0)
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

	// Command grid: "help:pg:<page>" and "help:c:<command>".
	switch {
	case strings.HasPrefix(parts[1], "pg:"):
		page, _ := strconv.Atoi(strings.TrimPrefix(parts[1], "pg:"))
		return showHelpPage(c, page)
	case strings.HasPrefix(parts[1], "c:"):
		return showHelpCommand(c, strings.TrimPrefix(parts[1], "c:"))
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
		return showHelpPage(c, 0)
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

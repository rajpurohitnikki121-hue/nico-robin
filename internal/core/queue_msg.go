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

package core

import (
	"sync"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	state "main/internal/core/models"
)

// queueEntry links a queued track to the "Added to Queue" confirmation
// message that was sent for it.
type queueEntry struct {
	seq uint64
	msg *telegram.NewMessage
}

var (
	queueMsgMu sync.Mutex
	queueMsgs  = make(map[*state.Track]*queueEntry)
	queueSeqs  = make(map[uint64]*state.Track)
	queueSeq   uint64
)

// RegisterQueueTrack reserves a short id for a queued track. The id is used in
// the callback data of the "Play Now" button (callback data is limited in size).
func RegisterQueueTrack(t *state.Track) uint64 {
	if t == nil {
		return 0
	}

	queueMsgMu.Lock()
	defer queueMsgMu.Unlock()

	if e, ok := queueMsgs[t]; ok {
		return e.seq
	}

	queueSeq++
	queueMsgs[t] = &queueEntry{seq: queueSeq}
	queueSeqs[queueSeq] = t
	return queueSeq
}

// SetQueueMsg remembers the queue confirmation message of a registered track.
func SetQueueMsg(t *state.Track, m *telegram.NewMessage) {
	if t == nil || m == nil {
		return
	}

	queueMsgMu.Lock()
	defer queueMsgMu.Unlock()

	if e, ok := queueMsgs[t]; ok {
		e.msg = m
	}
}

// QueueTrackBySeq returns the track registered under the given Play Now id.
func QueueTrackBySeq(seq uint64) *state.Track {
	queueMsgMu.Lock()
	defer queueMsgMu.Unlock()
	return queueSeqs[seq]
}

// DeleteQueueMsg forgets a track's queue confirmation message and deletes it
// from the chat. It is safe to call with a nil track or a track that has no
// queue message, and a failed deletion (missing rights, message already gone)
// is only logged.
func DeleteQueueMsg(t *state.Track) {
	if t == nil {
		return
	}

	queueMsgMu.Lock()
	e, ok := queueMsgs[t]
	if ok {
		delete(queueMsgs, t)
		delete(queueSeqs, e.seq)
	}
	queueMsgMu.Unlock()

	if !ok || e.msg == nil {
		return
	}

	go func(m *telegram.NewMessage) {
		if _, err := m.Delete(); err != nil {
			gologging.DebugF("queue message delete failed: %v", err)
		}
	}(e.msg)
}

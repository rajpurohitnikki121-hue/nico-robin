package modules

import (
	"sync"
	"time"

	"github.com/Laky-64/gologging"
	tg "github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	"main/internal/core"
	"main/internal/database"
)

const (
	cleanModeBatchWindow = 10 * time.Second
	defaultCleanDelay    = 15 * time.Minute

	// oldPlayerDelay is how long a replaced/finished player card stays before
	// it is removed. A card that is still the active player is never removed
	// (see flushDue), so this is only a short grace period.
	oldPlayerDelay = 15 * time.Second
	// queueFinishedDelay is how long the "Queue Finished!" message stays.
	queueFinishedDelay = 30 * time.Second
)

var (
	cleanModeDurationOptions = []int{15, 30, 60, 5}
	cleanScheduler           = &CleanScheduler{pending: make(map[int64][]cleanEntry)}
)

func cleanModeReadHandler(u tg.Update, _ *tg.Client) error {
	upd, ok := u.(*tg.UpdateReadChannelOutbox)
	if !ok || upd.MaxID == 0 {
		return nil
	}

	chatID := int64(-1000000000000 - upd.ChannelID)
	if upd.MaxID == currentPlayingStatusMessageID(chatID) {
		return nil
	}

	cleanScheduler.schedule(chatID, upd.MaxID)
	return nil
}

func currentPlayingStatusMessageID(chatID int64) int32 {
	room, ok := core.GetRoom(chatID, nil, false)
	if !ok || room == nil {
		return 0
	}
	status := room.StatusMsg()
	if status == nil {
		return 0
	}
	return status.ID
}

func cleanModeDelay(chatID int64) time.Duration {
	settings, err := database.GetChatSettings(chatID)
	if err != nil || settings.CleanModeDurationMins <= 0 {
		return defaultCleanDelay
	}
	return time.Duration(settings.CleanModeDurationMins) * time.Minute
}

type cleanEntry struct {
	messageID int32
	dueAt     time.Time
}

type CleanScheduler struct {
	mu      sync.Mutex
	pending map[int64][]cleanEntry
}

func (s *CleanScheduler) start() {
	go func() {
		ticker := time.NewTicker(cleanModeBatchWindow)
		defer ticker.Stop()
		for range ticker.C {
			s.flushDue(time.Now())
		}
	}()
}

func (s *CleanScheduler) schedule(chatID int64, messageID int32) {
	s.scheduleAfter(chatID, messageID, cleanModeDelay(chatID))
}

// scheduleAfter queues a message for deletion after the given delay.
func (s *CleanScheduler) scheduleAfter(
	chatID int64,
	messageID int32,
	delay time.Duration,
) {
	if messageID == 0 {
		return
	}
	// Never clean the logger group: its play logs must stay.
	if config.LoggerID != 0 && chatID == config.LoggerID {
		return
	}
	s.mu.Lock()
	s.pending[chatID] = append(s.pending[chatID], cleanEntry{
		messageID: messageID,
		dueAt:     time.Now().Add(delay),
	})
	s.mu.Unlock()
}

func (s *CleanScheduler) cancel(chatID int64) {
	s.mu.Lock()
	delete(s.pending, chatID)
	s.mu.Unlock()
}

func (s *CleanScheduler) flushDue(deadline time.Time) {
	s.mu.Lock()
	batches := make(map[int64][]int32)

	for chatID, entries := range s.pending {
		statusID := currentPlayingStatusMessageID(chatID)
		keep := entries[:0]

		for _, entry := range entries {
			if entry.messageID == statusID || entry.dueAt.After(deadline) {
				keep = append(keep, entry)
				continue
			}
			batches[chatID] = append(batches[chatID], entry.messageID)
		}

		if len(keep) == 0 {
			delete(s.pending, chatID)
		} else {
			s.pending[chatID] = keep
		}
	}
	s.mu.Unlock()

	for chatID, ids := range batches {
		if _, err := core.Bot.DeleteMessages(chatID, ids); err != nil {
			gologging.DebugF("cleanmode delete failed chat=%d err=%v", chatID, err)
		}
	}
}

func scheduleOldPlayingMessage(r *core.RoomState) {
	if m := r.StatusMsg(); m != nil {
		cleanScheduler.scheduleAfter(m.ChannelID(), m.ID, oldPlayerDelay)
	}
}

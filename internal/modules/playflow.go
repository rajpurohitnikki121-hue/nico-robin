package modules

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	state "main/internal/core/models"
	"main/internal/platforms"
)

// ---- download cancel registry (safe for concurrent use) ----

var (
	downloadCancelsMu sync.Mutex
	downloadCancels   = make(map[int64]func())
)

func setDownloadCancel(chatID int64, cancel func()) {
	downloadCancelsMu.Lock()
	downloadCancels[chatID] = cancel
	downloadCancelsMu.Unlock()
}

// clearDownloadCancel forgets the cancel func of a chat without calling it.
func clearDownloadCancel(chatID int64) {
	downloadCancelsMu.Lock()
	delete(downloadCancels, chatID)
	downloadCancelsMu.Unlock()
}

// cancelDownload cancels the running download of a chat, if there is one.
func cancelDownload(chatID int64) bool {
	downloadCancelsMu.Lock()
	cancel, ok := downloadCancels[chatID]
	delete(downloadCancels, chatID)
	downloadCancelsMu.Unlock()

	if ok {
		cancel()
	}
	return ok
}

// ---- one /play at a time per voice chat, first come first served ----

var (
	playLocksMu sync.Mutex
	playLocks   = make(map[int64]chan struct{})
)

// lockPlay waits until it is this request's turn for the room and returns the
// function that gives the turn to the next request. Waiting requests are
// served in the order they arrived, so songs are added in request order even
// when many people use /play at the same moment.
func lockPlay(roomID int64) (unlock func()) {
	playLocksMu.Lock()
	ch, ok := playLocks[roomID]
	if !ok {
		ch = make(chan struct{}, 1)
		playLocks[roomID] = ch
	}
	playLocksMu.Unlock()

	ch <- struct{}{} // blocked senders are woken up in FIFO order
	return func() { <-ch }
}

// ---- download with retries ----

const (
	downloadAttempts   = 3
	downloadRetryDelay = 2 * time.Second
)

// downloadWithRetry downloads a track, trying again when it fails, so a
// temporary error of the download service does not lose someone's song.
func downloadWithRetry(
	ctx context.Context,
	t *state.Track,
	statusMsg *telegram.NewMessage,
) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		path, err := platforms.Download(ctx, t, statusMsg)
		if err == nil {
			return path, nil
		}
		lastErr = err

		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return "", err
		}

		gologging.WarnF(
			"[download] attempt %d/%d failed for %q: %v",
			attempt, downloadAttempts, t.Title, err,
		)
		if attempt < downloadAttempts {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(downloadRetryDelay):
			}
		}
	}
	return "", lastErr
}

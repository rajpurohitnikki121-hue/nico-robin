package modules

import (
	"context"
	"sync"
	"time"

	"github.com/Laky-64/gologging"

	"main/internal/core"
	state "main/internal/core/models"
	"main/internal/platforms"
)

// Prefetch: while a song plays, the next queued song is downloaded in the
// background. Skipping, or the end of the song, then finds the file ready
// instead of waiting for the download.

var (
	prefetchMu sync.Mutex
	prefetchOn = map[string]chan struct{}{} // track id -> closed when done
)

func init() {
	core.OnTrackStarted = prefetchNext
}

func prefetchNext(r *core.RoomState) {
	if r == nil || r.IsDestroyed() || r.Shuffle() {
		return
	}
	var t *state.Track
	if q := r.Queue(); len(q) > 0 {
		t = q[0]
	} else {
		// Nothing queued: with autoplay on, pick the next song now.
		t = autoplayPrepare(r)
	}
	if t == nil || t.Video || t.ID == "" {
		return
	}

	prefetchMu.Lock()
	if _, busy := prefetchOn[t.ID]; busy {
		prefetchMu.Unlock()
		return
	}
	done := make(chan struct{})
	prefetchOn[t.ID] = done
	prefetchMu.Unlock()

	defer func() {
		prefetchMu.Lock()
		delete(prefetchOn, t.ID)
		prefetchMu.Unlock()
		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := platforms.Download(ctx, t, nil); err != nil {
		gologging.DebugF("[prefetch] %q not downloaded early: %v", t.Title, err)
		return
	}
	gologging.DebugF("[prefetch] %q is ready", t.Title)
}

// waitPrefetch blocks while the same song is being prefetched, so it is not
// downloaded twice at the same time.
func waitPrefetch(ctx context.Context, id string) {
	prefetchMu.Lock()
	done, busy := prefetchOn[id]
	prefetchMu.Unlock()
	if !busy {
		return
	}
	select {
	case <-done:
	case <-ctx.Done():
	}
}

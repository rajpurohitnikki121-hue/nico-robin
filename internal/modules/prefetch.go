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

	release, ok := tryLockTrack(t.ID)
	if !ok {
		return
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := platforms.Download(ctx, t, nil); err != nil {
		gologging.DebugF("[prefetch] %q not downloaded early: %v", t.Title, err)
		return
	}
	gologging.DebugF("[prefetch] %q is ready", t.Title)
}

// Per-song download lock: the same song is never downloaded twice at the same
// time (early download and a skip / song end asking for it together).

func tryLockTrack(id string) (release func(), ok bool) {
	prefetchMu.Lock()
	defer prefetchMu.Unlock()
	if _, busy := prefetchOn[id]; busy {
		return nil, false
	}
	done := make(chan struct{})
	prefetchOn[id] = done
	return func() {
		prefetchMu.Lock()
		delete(prefetchOn, id)
		prefetchMu.Unlock()
		close(done)
	}, true
}

// lockTrack waits until no other download of the song is running.
func lockTrack(ctx context.Context, id string) (release func(), err error) {
	for {
		if rel, ok := tryLockTrack(id); ok {
			return rel, nil
		}
		prefetchMu.Lock()
		done := prefetchOn[id]
		prefetchMu.Unlock()
		if done == nil {
			continue
		}
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

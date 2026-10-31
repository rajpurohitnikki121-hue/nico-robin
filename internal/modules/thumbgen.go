package modules

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	state "main/internal/core/models"
	"main/internal/utils"
)

// Player-card thumbnail: blurred artwork as background, rounded cover on the
// left, title / source / progress bar / controls on the right. It is composed
// with ffmpeg (already in the image) so no new Go dependency is needed. Any
// failure falls back to the plain artwork URL, so playback is never blocked.

const (
	thumbFont    = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
	thumbDir     = "downloads/thumbs"
	thumbTimeout = 12 * time.Second
	thumbMaxKeep = 60
)

var thumbMu sync.Mutex

// thumbMedia returns what should be passed as SendOptions.Media for a track.
func thumbMedia(t *state.Track) string {
	if t == nil || t.Artwork == "" {
		return ""
	}
	plain := utils.CleanURL(t.Artwork)
	if p := buildThumb(t); p != "" {
		return p
	}
	return plain
}

func buildThumb(t *state.Track) string {
	if _, err := os.Stat(thumbFont); err != nil {
		return ""
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return ""
	}

	sum := sha1.Sum([]byte(t.ID + "|" + t.Title + "|" + t.Artwork))
	key := hex.EncodeToString(sum[:8])
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return ""
	}
	out := filepath.Join(thumbDir, key+".jpg")
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out
	}

	thumbMu.Lock()
	defer thumbMu.Unlock()
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out
	}

	l1, l2 := wrapTitle(t.Title)
	sub := "YouTube"
	if t.Source != "" {
		sub = cleanGlyphs(string(t.Source))
		if sub == "" {
			sub = "Music"
		}
	}
	dur := utils.FormatTime(t.Duration)

	files := map[string]string{
		"l1": l1, "l2": l2, "sub": sub, "t0": "0:00", "t1": dur,
	}
	for name, txt := range files {
		if err := os.WriteFile(filepath.Join(thumbDir, key+"."+name+".txt"), []byte(txt), 0o644); err != nil {
			return ""
		}
	}
	defer func() {
		for name := range files {
			_ = os.Remove(filepath.Join(thumbDir, key+"."+name+".txt"))
		}
	}()

	tf := func(name string) string { return filepath.Join(thumbDir, key+"."+name+".txt") }
	dt := func(name, extra string) string {
		return "drawtext=fontfile=" + thumbFont + ":textfile=" + tf(name) + ":expansion=none:" + extra
	}

	const radius = "48"
	alpha := "if(lte(pow(max(max(" + radius + "-X,X-(W-1-" + radius + ")),0),2)+pow(max(max(" + radius + "-Y,Y-(H-1-" + radius + ")),0),2)," + radius + "*" + radius + "),255,0)"

	graph := strings.Join([]string{
		"[0:v]split=2[bgs][cvs]",
		"[bgs]scale=1280:720:force_original_aspect_ratio=increase,crop=1280:720,gblur=sigma=45,eq=brightness=-0.22:saturation=1.15[bg]",
		"[cvs]scale=500:500:force_original_aspect_ratio=decrease,pad=500:500:(ow-iw)/2:(oh-ih)/2:black,format=rgba,geq=r='r(X,Y)':g='g(X,Y)':b='b(X,Y)':a='" + alpha + "'[cv]",
		"[bg][cv]overlay=114:110:format=auto," + strings.Join([]string{
			dt("l1", "fontcolor=white:fontsize=46:x=718:y=118"),
			dt("l2", "fontcolor=white:fontsize=46:x=718:y=172"),
			dt("sub", "fontcolor=white@0.6:fontsize=28:x=718:y=240"),
			"drawbox=x=718:y=300:w=468:h=8:color=white@0.30:t=fill",
			"drawbox=x=718:y=300:w=40:h=8:color=white:t=fill",
			dt("t0", "fontcolor=white@0.7:fontsize=24:x=718:y=322"),
			dt("t1", "fontcolor=white@0.7:fontsize=24:x=1186-tw:y=322"),
			"drawtext=fontfile=" + thumbFont + ":text='◀◀':fontcolor=white:fontsize=64:x=770:y=392",
			"drawtext=fontfile=" + thumbFont + ":text='❚❚':fontcolor=white:fontsize=64:x=922:y=392",
			"drawtext=fontfile=" + thumbFont + ":text='▶▶':fontcolor=white:fontsize=64:x=1074:y=392",
			"drawbox=x=756:y=520:w=380:h=6:color=white@0.30:t=fill",
			"drawbox=x=756:y=520:w=140:h=6:color=white:t=fill",
		}, ","),
	}, ";")

	ctx, cancel := context.WithTimeout(context.Background(), thumbTimeout)
	defer cancel()

	tmp := out + ".tmp.jpg"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-loglevel", "error",
		"-i", utils.CleanURL(t.Artwork),
		"-filter_complex", graph,
		"-frames:v", "1", "-q:v", "3", tmp)
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return ""
	}
	if st, err := os.Stat(tmp); err != nil || st.Size() == 0 {
		_ = os.Remove(tmp)
		return ""
	}
	if err := os.Rename(tmp, out); err != nil {
		_ = os.Remove(tmp)
		return ""
	}
	pruneThumbs()
	return out
}

// cleanGlyphs keeps only characters DejaVu Sans can draw reliably (Latin,
// Greek, Cyrillic, digits, punctuation); everything else is dropped.
func cleanGlyphs(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case r < 0x20:
		case r < 0x250, r >= 0x370 && r < 0x530, r >= 0x2010 && r <= 0x2027:
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// wrapTitle splits the title into at most two lines of ~19 characters.
func wrapTitle(title string) (string, string) {
	const width = 19
	t := cleanGlyphs(strings.TrimSpace(title))
	if t == "" {
		t = "Unknown Title"
	}
	var lines [2]string
	cur := 0
	for _, w := range strings.Fields(t) {
		wr := []rune(w)
		if len(wr) > width {
			wr = wr[:width]
			w = string(wr)
		}
		cand := w
		if lines[cur] != "" {
			cand = lines[cur] + " " + w
		}
		if len([]rune(cand)) <= width {
			lines[cur] = cand
			continue
		}
		if cur == 1 {
			r := []rune(lines[1])
			if len(r) > width-3 {
				r = r[:width-3]
			}
			lines[1] = strings.TrimSpace(string(r)) + "..."
			return lines[0], lines[1]
		}
		cur = 1
		lines[1] = w
	}
	return lines[0], lines[1]
}

// pruneThumbs keeps the cache folder small.
func pruneThumbs() {
	entries, err := os.ReadDir(thumbDir)
	if err != nil || len(entries) <= thumbMaxKeep {
		return
	}
	type f struct {
		p string
		t time.Time
	}
	var list []f
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() {
			list = append(list, f{filepath.Join(thumbDir, e.Name()), info.ModTime()})
		}
	}
	for len(list) > thumbMaxKeep {
		oldest := 0
		for i := range list {
			if list[i].t.Before(list[oldest].t) {
				oldest = i
			}
		}
		_ = os.Remove(list[oldest].p)
		list = append(list[:oldest], list[oldest+1:]...)
	}
}

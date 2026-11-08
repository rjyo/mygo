//go:build !js

package e2e

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/wait"
)

// The responses of TestSchemeLargeBody and TestSchemeMediaRanges.
var schemeStreams struct {
	sync.Mutex
	big    context.Context // the context of the last /stream/big
	ranges []streamedRange // what /stream/tone.wav was asked and wrote
}

type streamedRange struct {
	header  string
	written int64
}

// toneWAV is a WAV file of 750 s of silence, 16-bit mono at 8 kHz: 12 MB,
// which no single response of WebView2's carries whole.
var toneWAV = sync.OnceValue(func() []byte {
	const rate, seconds = 8000, 750
	data := rate * 2 * seconds
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+data))
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(rate), uint32(rate * 2), uint16(2), uint16(16)} {
		binary.Write(&b, binary.LittleEndian, v) // PCM, mono, 8 kHz, 16-bit
	}
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(data))
	b.Write(make([]byte, data))
	return b.Bytes()
})

type countingWriter struct {
	http.ResponseWriter
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	c.n += int64(n)
	return n, err
}

// streamByte is byte i of /stream/big.
func streamByte(i int) byte { return byte(i*31 + i>>10) }

// useSchemeStreams serves /stream/big?n=N, N bytes written 32 KB at a
// time, and /stream/tone.wav, with range requests.
func useSchemeStreams(mux *http.ServeMux) {
	mux.HandleFunc("GET /stream/big", func(w http.ResponseWriter, r *http.Request) {
		schemeStreams.Lock()
		schemeStreams.big = r.Context()
		schemeStreams.Unlock()
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(n))
		buf := make([]byte, 32<<10)
		for i := 0; i < n; {
			k := min(len(buf), n-i)
			for j := range k {
				buf[j] = streamByte(i + j)
			}
			if _, err := w.Write(buf[:k]); err != nil {
				return
			}
			i += k
		}
	})
	mux.HandleFunc("GET /stream/tone.wav", func(w http.ResponseWriter, r *http.Request) {
		cw := &countingWriter{ResponseWriter: w}
		http.ServeContent(cw, r, "tone.wav", time.Time{}, bytes.NewReader(toneWAV()))
		schemeStreams.Lock()
		schemeStreams.ranges = append(schemeStreams.ranges, streamedRange{r.Header.Get("Range"), cw.n})
		schemeStreams.Unlock()
	})
}

// A body larger than what a backend holds of it streams to the page whole
// and in order; on Windows, the request's context ends once WebView2 read
// it and let it go.
func TestSchemeLargeBody(t *testing.T) {
	w := newWindow(t, mygo.WindowOptions{Title: "Large body", Width: 300, Height: 200})
	if err := w.Page().LoadURL("app://localhost/"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, w, `window.run && document.readyState === "complete"`)
	const n = 12<<20 + 12345
	var want uint32
	for i := range n {
		want += uint32(streamByte(i))
	}
	got, err := mygo.EvalAs[string](w.Page(), fmt.Sprintf(`const r = await fetch("/stream/big?n=%d");
		const b = new Uint8Array(await r.arrayBuffer());
		let sum = 0;
		for (let i = 0; i < b.length; i++) sum = (sum + b[i]) >>> 0;
		return r.status + " " + b.length + " " + sum`, n))
	if err != nil {
		t.Fatal(err)
	}
	if w := fmt.Sprintf("200 %d %d", n, want); got != w {
		t.Fatalf("the page read %q, want %q (status, length, sum of the bytes)", got, w)
	}
	if runtime.GOOS == "windows" {
		schemeStreams.Lock()
		ctx := schemeStreams.big
		schemeStreams.Unlock()
		wait.Until(t, "the request's context to end once WebView2 read the body", func() bool { return ctx.Err() != nil })
	}
}

// A media element asking for the rest of a file gets parts of it on
// Windows, a few MB each, and asks for the next as it needs them: the
// handler never writes the whole file.
func TestSchemeMediaRanges(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("WebKitGTK plays no media from a custom scheme (docs/frontend.md)")
	}
	w := newWindow(t, mygo.WindowOptions{Title: "Media ranges", Width: 300, Height: 200})
	if err := w.Page().LoadURL("app://localhost/"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, w, `window.run && document.readyState === "complete"`)
	schemeStreams.Lock()
	schemeStreams.ranges = nil
	schemeStreams.Unlock()
	const events = `const once = (a, type) => new Promise((ok, fail) => {
		a.addEventListener(type, ok, {once: true});
		a.addEventListener("error", () => fail(new Error("media error " + a.error.code + " " + a.error.message)), {once: true});
	});`
	got, err := mygo.EvalAs[float64](w.Page(), events+`
		const a = document.createElement("audio");
		a.preload = "auto";
		a.src = "/stream/tone.wav";
		document.body.append(a);
		window.streamAudio = a;
		if (a.readyState < 1) await once(a, "loadedmetadata");
		return a.duration`)
	if err != nil {
		t.Fatalf("loading the audio: %v", err)
	}
	if got < 749 || got > 751 {
		t.Errorf("duration %v s, want 750", got)
	}
	if _, err := mygo.EvalAs[bool](w.Page(), events+`
		const a = window.streamAudio;
		a.currentTime = 600;
		await once(a, "seeked");
		return true`); err != nil {
		t.Fatalf("seeking to 600 s: %v", err)
	}
	if runtime.GOOS != "windows" {
		return
	}
	schemeStreams.Lock()
	ranges := append([]streamedRange(nil), schemeStreams.ranges...)
	schemeStreams.Unlock()
	seeked := false
	for _, r := range ranges {
		first, last, ok := strings.Cut(strings.TrimPrefix(r.header, "bytes="), "-")
		start, err1 := strconv.ParseInt(first, 10, 64)
		end, err2 := strconv.ParseInt(last, 10, 64)
		if !ok || err1 != nil || err2 != nil || end-start+1 > 4<<20 {
			t.Errorf("the handler was asked for %q, not a range of at most 4 MB", r.header)
		}
		if r.written > 4<<20 {
			t.Errorf("the handler wrote %d bytes for %q", r.written, r.header)
		}
		if start > 9<<20 {
			seeked = true
		}
	}
	if !seeked {
		t.Errorf("no range from 600 s on (byte 9.6 MB) after the seek: %+v", ranges)
	}
	t.Logf("ranges: %+v", ranges)
}

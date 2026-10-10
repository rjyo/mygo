//go:build darwin

package darwin

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/transfer"
)

// dataDragClassesOnce registers the drag classes once for the tests using them.
var dataDragClassesOnce sync.Once

func TestFilePromiseWriter(t *testing.T) {
	load()
	withPool(func() {
		dataDragClassesOnce.Do(registerDataDragClasses)
		wrote := make(chan string, 1)
		data := transfer.New(transfer.NewItem(
			transfer.Promise("notes.txt", func(path string) error { wrote <- path; return errors.New("offline") }),
			transfer.Bytes(transfer.Text, []byte("notes.txt")),
		))
		src := &macDataSource{r: platform.DragRequest{Data: data, Session: "token"}}
		obj := src.pasteboardItems()[0]
		p := macPromises[obj]
		if p == nil || !sendBool(obj, "isKindOfClass:", uintptr(class("NSFilePromiseProvider"))) {
			t.Fatal("promise item is not a file promise provider")
		}
		if got := goString(send(obj, "fileType")); got != "public.plain-text" {
			t.Errorf("file type %q", got)
		}
		var types []string
		for _, typ := range arrayItems(send(obj, "writableTypesForPasteboard:", 0)) {
			types = append(types, goString(typ))
		}
		for _, want := range []string{macType(transfer.Text), macType(platform.DragSessionFormat)} {
			if !slices.Contains(types, want) {
				t.Errorf("writable types %v lack %s", types, want)
			}
		}
		if len(types) < 3 {
			t.Errorf("super's promise types are missing: %v", types)
		}
		session := goBytes(send(obj, "pasteboardPropertyListForType:", uintptr(nsString(macType(platform.DragSessionFormat)))))
		if string(session) != "token" {
			t.Errorf("session %q", session)
		}
		if got := goString(send(obj, "filePromiseProvider:fileNameForType:", uintptr(obj), 0)); got != "notes.txt" {
			t.Errorf("file name %q", got)
		}

		main := make(chan func(), 1)
		promiseMain = func(fn func()) { main <- fn }
		defer func() { promiseMain = func(fn func()) { theBackend.post(fn) } }()
		done := make(chan string, 1)
		blk := newBlock(func(_ objc.Block, err id) { done <- goString(send(err, "localizedDescription")) })
		defer blk.Release()
		send(obj, "filePromiseProvider:writePromiseToURL:completionHandler:", uintptr(obj), uintptr(fileURL("/tmp/drop/notes.txt")), uintptr(blk))
		if got := <-wrote; got != "/tmp/drop/notes.txt" {
			t.Errorf("wrote %q", got)
		}
		src.r.Done = func(transfer.Result) {}
		src.obj = send(send(class("MyGoDragSource"), "alloc"), "init")
		src.s = &surface{}
		src.finish(1)
		if macPromises[obj] == nil {
			t.Fatal("provider released while the destination waits")
		}
		// The completion comes back through the main thread.
		(<-main)()
		if msg := <-done; msg != "offline" {
			t.Errorf("completion error %q", msg)
		}
		if macPromises[obj] != nil {
			t.Error("provider kept after the write")
		}
	})
}

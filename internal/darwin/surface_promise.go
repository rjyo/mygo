//go:build darwin

package darwin

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/transfer"
)

// A file promise (transfer.Promise) drags as an NSFilePromiseProvider. The
// subclass also writes the item's other formats and the drag session, so
// the drag still reaches typed targets in this process.

type macPromise struct {
	src   *macDataSource
	obj   id
	item  transfer.Item
	name  string
	write func(path string) error
	types []string
	// writing until the destination's completion handler ran; over once
	// the drag session ended. The provider lives until both are done.
	writing, over bool
}

var macPromises = map[id]*macPromise{}

// promiseMain runs a finished write's completion on the main thread.
var promiseMain = func(fn func()) { theBackend.post(fn) }

const promiseClass = "MyGoFilePromise"

func registerFilePromiseClass() {
	classDef(promiseClass, "NSFilePromiseProvider", []string{"NSFilePromiseProviderDelegate"}, []objc.MethodDef{
		method("writableTypesForPasteboard:", func(self id, cmd objc.SEL, board id) id {
			types := sendSuper(self, promiseClass, cmd, uintptr(board))
			p := macPromises[self]
			if p == nil {
				return types
			}
			var extra []id
			for _, t := range p.types {
				extra = append(extra, nsString(t))
			}
			return send(types, "arrayByAddingObjectsFromArray:", uintptr(nsArray(extra...)))
		}),
		method("pasteboardPropertyListForType:", func(self id, cmd objc.SEL, typ id) id {
			p := macPromises[self]
			name := goString(typ)
			if p == nil || !slices.Contains(p.types, name) {
				return sendSuper(self, promiseClass, cmd, uintptr(typ))
			}
			b, err := p.src.itemData(p.item, name)
			if err != nil {
				p.src.err = err
				return 0
			}
			return nsData(b)
		}),
		method("filePromiseProvider:fileNameForType:", func(self id, _ objc.SEL, provider, typ id) id {
			if p := macPromises[self]; p != nil {
				return nsString(p.name)
			}
			return 0
		}),
		// AppKit asks on the main queue (no operationQueueForFilePromiseProvider:);
		// the writer runs on its own goroutine, completing when it returns.
		method("filePromiseProvider:writePromiseToURL:completionHandler:", func(self id, _ objc.SEL, provider, url id, handler uintptr) {
			p := macPromises[self]
			if p == nil {
				callBlock(handler, uintptr(promiseError(fmt.Errorf("mygo: the drag is over"))))
				return
			}
			path := goString(send(url, "path"))
			handler = blockCopy(handler)
			p.writing = true
			go func() {
				err := p.run(path)
				promiseMain(func() {
					withPool(func() {
						var e id
						if err != nil {
							e = promiseError(err)
						}
						callBlock(handler, uintptr(e))
						blockRelease(handler)
					})
					p.writing = false
					p.release()
				})
			}()
		}),
	})
}

// filePromise makes the pasteboard writer for an item's promise.
func (src *macDataSource) filePromise(item transfer.Item, name string, write func(string) error) id {
	// A drop no destination asked to write (one in this process) left its
	// provider; a new drag means nobody will.
	for _, p := range macPromises {
		if p.over && !p.writing && p.src != src {
			p.drop()
		}
	}
	// The provider is its own delegate; init checks for its methods.
	obj := send(class(promiseClass), "alloc")
	obj = send(obj, "initWithFileType:delegate:", uintptr(nsString(promiseFileType(name))), uintptr(obj))
	p := &macPromise{src: src, obj: obj, item: item, name: name, write: write, types: itemTypes(item)}
	macPromises[obj] = p
	src.promises = append(src.promises, p)
	return obj
}

func (p *macPromise) run(path string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", transfer.ErrProviderPanic, r)
		}
	}()
	return p.write(path)
}

// ended is the drag session's end. A canceled drag writes nothing; a
// dropped one keeps the provider for the destination's request, which
// may come after the session ends.
func (p *macPromise) ended(canceled bool) {
	p.over = true
	if canceled {
		p.release()
	}
}

func (p *macPromise) release() {
	if p.over && !p.writing {
		p.drop()
	}
}

func (p *macPromise) drop() {
	if macPromises[p.obj] == p {
		delete(macPromises, p.obj)
		release(p.obj)
	}
}

// promiseFileType is the file's type identifier, from its extension.
func promiseFileType(name string) string {
	if ext := strings.TrimPrefix(filepath.Ext(name), "."); ext != "" {
		if cls := class("UTType"); cls != 0 {
			if ut := send(cls, "typeWithFilenameExtension:", uintptr(nsString(ext))); ut != 0 {
				return goString(send(ut, "identifier"))
			}
		}
	}
	return "public.data"
}

func promiseError(err error) id {
	info := send(class("NSDictionary"), "dictionaryWithObject:forKey:", uintptr(nsString(err.Error())), uintptr(nsString("NSLocalizedDescription")))
	return send(class("NSError"), "errorWithDomain:code:userInfo:", uintptr(nsString("dev.mygo.transfer")), 1, uintptr(info))
}

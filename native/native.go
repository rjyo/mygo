// Package native runs native modules: Go interfaces implemented in a
// platform's own language, Swift on macOS (see docs/native-modules.md).
//
// A module's package declares its interfaces, and `mygo generate` writes
// the rest: the constructors and the values that call the implementation,
// which use this package. An app calls a module's constructor, NewT, and
// never this package directly, except for its types: errors made with
// NewError, Buffer and View.
package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/egoist/mygo"
)

// ErrClosed is returned by the methods of an object that was closed.
var ErrClosed = errors.New("native: object closed")

// Error is an error an implementation returned. The errors a module
// declares are made with NewError and compare by their code, so
// errors.Is(err, language.ErrUndetermined) holds for the error the
// platform threw as LanguageError.undetermined. Any other error the
// platform throws has no code, and says what the platform said of it.
type Error struct {
	// Code is the code of an error the module declares, else empty.
	Code string
	// Domain is the error's domain or type, as NSCocoaErrorDomain or
	// ParseError, for an error the module does not declare.
	Domain string
	// Number is the error's code in its domain.
	Number int
	// Message describes the error.
	Message string
}

// NewError makes an error a module declares, which its implementations
// throw by code: in Swift, a case of the enum <Module>Error.
func NewError(code string) *Error { return &Error{Code: code} }

func (e *Error) Error() string {
	switch {
	case e.Message != "":
		return e.Message
	case e.Domain != "":
		return fmt.Sprintf("%s error %d", e.Domain, e.Number)
	}
	return e.Code
}

// Is reports whether target is an error the module declares with the same
// code.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code != "" && t.Code == e.Code
}

// View is embedded by the interfaces of a module that a view of the
// platform implements, which ui.NativeView shows in native UI.
type View interface {
	// NativeView returns the platform's view: an NSView* on macOS. It is
	// 0 once the view is closed, which takes it out of the windows
	// showing it.
	NativeView() uintptr
}

// object is what every value a module returns implements.
type object interface {
	NativeObject() *Object
}

// Module describes a module to the runtime. Generated code declares one
// per package; its fields are not for apps.
type Module struct {
	// Name is the Swift module's: Language.
	Name string
	// Symbol prefixes the functions the implementation exports: language.
	Symbol string
	// Dir is the directory of the package, when the binary knows it
	// (built without -trimpath), where `mygo generate` builds the
	// implementation for go test and go run.
	Dir string
	// MinMacOS is the oldest macOS the implementation runs on, from the
	// platforms of its Package.swift.
	MinMacOS string
	// Errors are the errors the module declares.
	Errors []*Error
	// Deps are the Swift modules of the native modules whose types it
	// uses, which load before it.
	Deps []string

	once sync.Once
	lib  *library
	err  error
}

// Wrapper makes the value of an interface of the platform's around an
// object, for other modules returning it.
type Wrapper struct {
	Flags Flags
	Wrap  func(*Object) any
}

var registry struct {
	sync.Mutex
	modules map[string]*Module
	wraps   map[string]map[string]Wrapper
}

// Register records a module and how to wrap the values of its interfaces,
// for the modules that use them. Generated code calls it as the package
// initializes.
func Register(m *Module, wraps map[string]Wrapper) {
	registry.Lock()
	defer registry.Unlock()
	if registry.modules == nil {
		registry.modules = map[string]*Module{}
		registry.wraps = map[string]map[string]Wrapper{}
	}
	registry.modules[m.Name] = m
	registry.wraps[m.Name] = wraps
}

// Wrap returns the value of the interface iface of the module named module
// around the handle h, which another module returned, or nil.
func Wrap(h uint64, module, iface string) any {
	registry.Lock()
	m, w := registry.modules[module], registry.wraps[module][iface]
	registry.Unlock()
	if h == 0 || m == nil || w.Wrap == nil {
		return nil
	}
	return w.Wrap(m.object(h, w.Flags))
}

// registered returns the module named name, as Register recorded it.
func registered(name string) *Module {
	registry.Lock()
	defer registry.Unlock()
	return registry.modules[name]
}

// Flags say how a method is called.
type Flags int

const (
	// MainThread runs the method on the main thread.
	MainThread Flags = 1 << iota
)

// Object is a value of the platform: what a constructor made, or a method
// returned. Its methods are for generated code.
type Object struct {
	m      *Module
	h      uint64
	closed atomic.Bool
	// funcs are the callbacks passed to the call that returned it, for a
	// subscription, released when it closes.
	funcs []uint64
	flags Flags
}

// View returns the object's view, for the NativeView of the proxies of
// views: the object itself, or 0 once closed.
func (o *Object) View() uintptr {
	if o == nil || o.closed.Load() {
		return 0
	}
	return uintptr(o.h)
}

// Handle returns the object's handle, to pass it back to the platform.
func (o *Object) Handle() uint64 {
	if o == nil {
		return 0
	}
	return o.h
}

// Module returns the module the object belongs to.
func (o *Object) Module() *Module { return o.m }

// object wraps a handle the platform returned, released once the Go value
// is unreachable unless closed before. Objects of flags MainThread are
// released on the main thread.
func (m *Module) object(h uint64, flags Flags) *Object {
	if h == 0 {
		return nil
	}
	o := &Object{m: m, h: h, flags: flags}
	type gone struct {
		h     uint64
		flags Flags
	}
	runtime.AddCleanup(o, func(g gone) { m.release(g.h, g.flags) }, gone{h, flags})
	return o
}

// release lets go of the object, on the main thread for one of the main
// thread's.
func (m *Module) release(h uint64, flags Flags) {
	lib, err := m.load()
	if err != nil {
		return
	}
	if flags&MainThread != 0 {
		// Posted, never waited for: the runtime runs cleanups on one
		// goroutine. Before the app runs, it waits for it to.
		mygo.App.WhenReady(func() {
			viewGone(uintptr(h))
			lib.release(m, h)
		})
		return
	}
	lib.release(m, h)
}

// New makes the implementation of an entry point.
func (m *Module) New(entry string, flags Flags) (*Object, error) {
	var h uint64
	if err := m.call(context.Background(), 0, "new_"+entry, flags, nil, &h); err != nil {
		return nil, err
	}
	if h == 0 {
		return nil, fmt.Errorf("native: %s's make%s returned nothing", m.Name, entry)
	}
	return m.object(h, flags), nil
}

// Object wraps the handle of an object the platform returned, whose
// methods flags describe.
func (m *Module) Object(h uint64, flags Flags) *Object { return m.object(h, flags) }

// Call calls a method of the object that returns at once, decoding what it
// returns into result unless nil.
func (o *Object) Call(sym string, flags Flags, args []any, result any) error {
	if o.closed.Load() {
		return ErrClosed
	}
	err := o.m.call(context.Background(), o.h, sym, flags, args, result)
	runtime.KeepAlive(o)
	return err
}

// CallContext calls an asynchronous method of the object, which ctx
// cancels.
func (o *Object) CallContext(ctx context.Context, sym string, flags Flags, args []any, result any) error {
	if o.closed.Load() {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := o.m.callAsync(ctx, o.h, sym, flags, args, result)
	runtime.KeepAlive(o)
	return err
}

// Subscribe calls a method that returns a Closer, which stops what it
// started and releases the callbacks funcs, those the call passed.
func (o *Object) Subscribe(sym string, flags Flags, args []any, funcs ...uint64) io.Closer {
	var h uint64
	err := o.Call(sym, flags, args, &h)
	if err != nil || h == 0 {
		// The platform kept none of them.
		Forget(funcs...)
		return nopCloser{err}
	}
	c := o.m.object(h, flags)
	c.funcs = funcs
	return c
}

type nopCloser struct{ err error }

func (c nopCloser) Close() error { return c.err }

// Close closes the object: the platform's close(), then its release. Later
// calls return ErrClosed.
func (o *Object) Close() error {
	if o == nil || !o.closed.CompareAndSwap(false, true) {
		return ErrClosed
	}
	var err error
	if o.flags&MainThread != 0 {
		var cerr error
		err = onMain(o.flags, func() {
			viewGone(uintptr(o.h))
			cerr = o.m.call(context.Background(), o.h, "close", o.flags, nil, nil)
		})
		if err == nil {
			err = cerr
		}
	} else {
		err = o.m.call(context.Background(), o.h, "close", 0, nil, nil)
	}
	releaseFuncs(o.funcs)
	runtime.KeepAlive(o)
	return err
}

// Stream calls a method that returns a stream, as an iterator that starts
// it. prepare makes the call's arguments as the loop starts, and returns
// the handles of the callbacks and values of Go's among them, which are
// forgotten if the stream does not start; keep are the arguments that
// stand for memory or objects of the platform's, kept reachable until the
// stream ends.
func Stream[T any](ctx context.Context, o *Object, sym string, flags Flags, prepare func() ([]any, []uint64), keep ...any) iter.Seq2[T, error] {
	return StreamMap(ctx, o, sym, flags, prepare, func(v T) T { return v }, keep...)
}

// StreamMap is Stream for values that cross as D and are given to Go as T.
func StreamMap[D, T any](ctx context.Context, o *Object, sym string, flags Flags, prepare func() ([]any, []uint64), conv func(D) T, keep ...any) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		defer runtime.KeepAlive(keep)
		var zero T
		if o.closed.Load() {
			yield(zero, ErrClosed)
			return
		}
		if err := ctx.Err(); err != nil {
			yield(zero, err)
			return
		}
		args, funcs := prepare()
		s, err := o.m.startStream(o.h, sym, flags, args)
		if err != nil {
			Forget(funcs...)
			yield(zero, err)
			return
		}
		defer s.stop()
		for {
			data, err, done := s.next(ctx)
			if done {
				return
			}
			if err != nil {
				yield(zero, err)
				return
			}
			var d D
			if err := json.Unmarshal(data, &d); err != nil {
				yield(zero, fmt.Errorf("native: %s: %w", sym, err))
				return
			}
			if !yield(conv(d), nil) {
				return
			}
		}
	}
}

// Seq turns a stream of values that cannot fail into an iter.Seq.
func Seq[T any](s iter.Seq2[T, error]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v, err := range s {
			if err != nil || !yield(v) {
				return
			}
		}
	}
}

// Args are the arguments the platform passes to Go: to a callback, or to a
// method of an interface Go implements.
type Args []json.RawMessage

// Decode decodes the argument i into v.
func (a Args) Decode(i int, v any) error {
	if i >= len(a) {
		return fmt.Errorf("native: argument %d is missing", i)
	}
	return json.Unmarshal(a[i], v)
}

// Func registers a callback, which the platform calls with its arguments,
// and returns its handle. A notification (notify) returns nothing and runs
// later, in order, on the main thread when the app runs; a question runs
// at once on the thread that asks and returns its answer.
func Func(fn func(Args) (any, error), notify bool) uint64 {
	return registerGo(&goValue{fn: fn, notify: notify})
}

// Export registers a value of an interface Go implements, which the
// platform calls through dispatch, and returns its handle. dispatch calls
// its method number method with args; ctx is that of a method taking one.
func Export(m *Module, v any, dispatch func(ctx context.Context, method int, args Args) (any, error)) uint64 {
	if v == nil {
		return 0
	}
	return registerGo(&goValue{m: m, v: v, dispatch: dispatch})
}

// goValue is a callback or an object Go implements that the platform
// holds.
type goValue struct {
	m        *Module
	fn       func(Args) (any, error)
	notify   bool
	v        any
	dispatch func(ctx context.Context, method int, args Args) (any, error)
	dead     atomic.Bool
}

var (
	goValues sync.Map // uint64 → *goValue
	nextID   atomic.Uint64
)

func registerGo(v *goValue) uint64 {
	id := nextID.Add(1)
	goValues.Store(id, v)
	return id
}

func lookupGo(id uint64) *goValue {
	v, ok := goValues.Load(id)
	if !ok {
		return nil
	}
	return v.(*goValue)
}

// releaseFuncs marks callbacks dead: the platform calling them does
// nothing. They go once the platform releases them.
func releaseFuncs(ids []uint64) {
	for _, id := range ids {
		if v := lookupGo(id); v != nil {
			v.dead.Store(true)
		}
	}
}

// errJSON is how errors cross, both ways.
type errJSON struct {
	Code        string `json:"code,omitempty"`
	Domain      string `json:"domain,omitempty"`
	Number      int    `json:"errCode,omitempty"`
	Message     string `json:"message,omitempty"`
	Unsupported bool   `json:"unsupported,omitempty"`
	Canceled    bool   `json:"canceled,omitempty"`
	Closed      bool   `json:"closed,omitempty"`
}

// decodeError turns an error the platform threw into Go's.
func (m *Module) decodeError(ctx context.Context, data []byte) error {
	var e errJSON
	if err := json.Unmarshal(data, &e); err != nil {
		return fmt.Errorf("native: %s threw an error that does not decode: %s", m.Name, data)
	}
	switch {
	case e.Unsupported:
		return errors.ErrUnsupported
	case e.Closed:
		return ErrClosed
	case e.Canceled:
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		return context.Canceled
	case e.Code != "":
		for _, d := range m.Errors {
			if d.Code == e.Code {
				return d
			}
		}
		return &Error{Code: e.Code, Message: e.Message}
	}
	return &Error{Domain: e.Domain, Number: e.Number, Message: e.Message}
}

// encodeError turns a Go error into what the platform throws.
func encodeError(err error) []byte {
	var e errJSON
	var ne *Error
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		e.Canceled = true
	case errors.Is(err, errors.ErrUnsupported):
		e.Unsupported = true
	case errors.As(err, &ne) && ne.Code != "":
		e.Code = ne.Code
	}
	e.Message = err.Error()
	data, _ := json.Marshal(e)
	return data
}

func encodeArgs(args []any) ([]byte, error) {
	if args == nil {
		return nil, nil
	}
	return json.Marshal(args)
}

// bytesPointer returns the address of data's first byte, or 0.
func bytesPointer(data []byte) uintptr {
	if len(data) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&data[0]))
}

// HandleOf returns the handle of v when a module returned it, else 0: a
// value Go made cannot stand for the platform's.
func HandleOf(v any) uint64 {
	if n, ok := v.(object); ok && n != nil {
		return n.NativeObject().Handle()
	}
	return 0
}

// WindowHandle returns the native window of w, NSWindow* on macOS, or 0.
func WindowHandle(w *mygo.Window) uint64 {
	if w == nil {
		return 0
	}
	return uint64(w.NativeHandle())
}

// Forget releases callbacks and values of Go's that a call failed to give
// the platform.
func Forget(ids ...uint64) {
	for _, id := range ids {
		goValues.Delete(id)
	}
}

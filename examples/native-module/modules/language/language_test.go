package language_test

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/examples/native-module/modules/language"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/native"
)

// analyzer makes the module's implementation, skipping where it is not
// built: run `mygo dev` in examples/native-module first, on macOS.
func analyzer(t *testing.T) language.Analyzer {
	t.Helper()
	a, err := language.NewAnalyzer()
	if errors.Is(err, errors.ErrUnsupported) {
		t.Skip("no implementation on this platform")
	}
	if err != nil && strings.Contains(err.Error(), "mygo native build") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestDetectLanguage(t *testing.T) {
	a := analyzer(t)
	lang, err := a.DetectLanguage("Bonjour tout le monde, comment allez-vous ?")
	if err != nil || lang != "fr" {
		t.Fatalf("DetectLanguage = %q, %v; want fr", lang, err)
	}
	if _, err := a.DetectLanguage(""); !errors.Is(err, language.ErrUndetermined) {
		t.Fatalf("DetectLanguage of nothing: %v; want ErrUndetermined", err)
	}
	h, err := a.Hypotheses("Hello there, how are you doing today?", 3)
	if err != nil || h["en"] <= 0.5 {
		t.Fatalf("Hypotheses = %v, %v", h, err)
	}
}

func TestTokens(t *testing.T) {
	a := analyzer(t)
	text := "Héllo world. Second sentence here."
	var got []language.Token
	for tok, err := range a.Tokens(wait.Context(t), text, language.Sentence) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, tok)
	}
	if len(got) != 2 || text[got[1].Start:got[1].End] != got[1].Text || !strings.HasPrefix(got[1].Text, "Second") {
		t.Fatalf("Tokens = %+v", got)
	}
	// Breaking stops the stream.
	n := 0
	for range a.Tokens(wait.Context(t), strings.Repeat("word ", 1000), language.Word) {
		if n++; n == 3 {
			break
		}
	}
	if n != 3 {
		t.Fatalf("got %d words before breaking", n)
	}
}

type ranker struct{}

func (ranker) Score(word string) float64 { return -float64(len(word)) }

func (ranker) Explain(ctx context.Context, word string) (string, error) {
	return "because " + word + " is short", nil
}

func TestObjects(t *testing.T) {
	a := analyzer(t)
	ctx := wait.Context(t)
	if _, err := a.Embedding(ctx, "xx"); !errors.Is(err, language.ErrNoEmbedding) {
		t.Fatalf("Embedding(xx): %v; want ErrNoEmbedding", err)
	}
	e, err := a.Embedding(ctx, "en")
	if err != nil {
		t.Skip("no English embedding:", err)
	}
	if d := e.Distance("coffee", "coffee"); d != 0 {
		t.Errorf("Distance to itself = %v", d)
	}
	if n := e.Neighbors("coffee", 5); len(n) != 5 {
		t.Errorf("Neighbors = %v", n)
	}
	// The embedding goes back to the module, and the ranker, which Go
	// implements, to the platform.
	var words []string
	for w, err := range a.Suggest(ctx, e, "coffee", ranker{}) {
		if err != nil {
			t.Fatal(err)
		}
		words = append(words, w)
	}
	if len(words) < 2 || !strings.HasPrefix(words[len(words)-1], "because ") || len(words[0]) > len(words[1]) {
		t.Fatalf("Suggest = %q", words)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); !errors.Is(err, native.ErrClosed) {
		t.Fatalf("second Close: %v; want ErrClosed", err)
	}
}

// An object passed to a stream stays alive until the stream ends, though
// the caller keeps no reference to it.
func TestObjectArgumentKeptAlive(t *testing.T) {
	a := analyzer(t)
	ctx := wait.Context(t)
	embedding := func() language.Embedding {
		e, err := a.Embedding(ctx, "en")
		if err != nil {
			t.Skip("no English embedding:", err)
		}
		return e
	}
	for range 5 {
		n := 0
		for w, err := range a.Suggest(ctx, embedding(), "coffee", ranker{}) {
			if err != nil {
				t.Fatal(err)
			}
			if w == "" {
				t.Fatal("an empty suggestion")
			}
			n++
			runtime.GC()
			runtime.GC()
		}
		if n < 2 {
			t.Fatalf("%d suggestions", n)
		}
	}
}

func TestCallbacks(t *testing.T) {
	a := analyzer(t)
	var mu sync.Mutex
	var words []string
	done := make(chan struct{})
	n := a.Count("one two three", func(word string, i int) {
		mu.Lock()
		defer mu.Unlock()
		if i != len(words) {
			t.Errorf("word %d came as %d", len(words), i)
		}
		if words = append(words, word); len(words) == 3 {
			close(done)
		}
	})
	if n != 3 {
		t.Fatalf("Count = %d", n)
	}
	wait.For(t, "the notifications", done)
	if strings.Join(words, " ") != "one two three" {
		t.Fatalf("words = %q", words)
	}

	ticks := make(chan int, 100)
	sub := a.Ticks(time.Millisecond, func(n int) { ticks <- n })
	for want := 1; want <= 3; want++ {
		if got := wait.For(t, "a tick", ticks); got != want {
			t.Fatalf("tick %d, want %d", got, want)
		}
	}
	if err := sub.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBufferStream(t *testing.T) {
	a := analyzer(t)
	var got strings.Builder
	addrs := map[*byte]bool{}
	for b, err := range a.Chunks(wait.Context(t), "abcdefghij", 4) {
		if err != nil {
			t.Fatal(err)
		}
		got.WriteString(strings.TrimRight(string(b.Bytes()), "\x00"))
		addrs[&b.Bytes()[0]] = true
	}
	if got.String() != "abcdefghij" {
		t.Fatalf("Chunks = %q", got.String())
	}
	t.Logf("3 chunks in %d buffers", len(addrs))
}

func TestBufferAndContext(t *testing.T) {
	a := analyzer(t)
	buf := native.NewBuffer(5)
	n, err := a.Uppercase("hello world", buf)
	if err != nil || n != 5 || string(buf.Bytes()) != "HELLO" {
		t.Fatalf("Uppercase = %d, %v, %q", n, err, buf.Bytes())
	}
	if err := a.Sleep(wait.Context(t), time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(wait.Context(t))
	cancel()
	if err := a.Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("Sleep canceled: %v", err)
	}
	ctx, cancel = context.WithCancel(wait.Context(t))
	errc := make(chan error, 1)
	go func() { errc <- a.Sleep(ctx, time.Hour) }()
	cancel()
	if err := wait.For(t, "Sleep to stop", errc); !errors.Is(err, context.Canceled) {
		t.Fatalf("Sleep canceled while waiting: %v", err)
	}
}

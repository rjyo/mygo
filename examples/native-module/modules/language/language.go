// Package language is a native module: natural language processing with
// Apple's NaturalLanguage framework, behind Go interfaces. Its
// implementation is the Swift package in apple/.
package language

import (
	"context"
	"io"
	"iter"
	"time"

	"github.com/egoist/mygo/native"
)

// Analyzer reads natural language text.
//
//mygo:native
type Analyzer interface {
	// DetectLanguage returns the BCP 47 code of the text's language, such
	// as "en" or "zh-Hans".
	DetectLanguage(text string) (string, error)
	// Hypotheses returns the languages the text may be in, with their
	// probabilities, at most max of them.
	Hypotheses(text string, max int) (map[string]float64, error)
	// Tokens yields the words, sentences or paragraphs of the text.
	Tokens(ctx context.Context, text string, unit Unit) iter.Seq2[Token, error]
	// Embedding loads the word embedding of a language.
	Embedding(ctx context.Context, language string) (Embedding, error)
	// Suggest yields the neighbors of word in the embedding, best first by
	// the ranker's scores.
	Suggest(ctx context.Context, e Embedding, word string, r Ranker) iter.Seq2[string, error]
	// Count calls progress with each word of the text it counts, and
	// returns how many there are.
	Count(text string, progress func(word string, index int)) int
	// Ticks calls f every interval until the subscription is closed.
	Ticks(interval time.Duration, f func(n int)) io.Closer
	// Uppercase writes the upper case of text into buf, and returns how
	// many bytes it wrote.
	Uppercase(text string, buf *native.Buffer) (int, error)
	// Chunks yields the UTF-8 bytes of text, size at a time, the last
	// chunk padded with zeros, in buffers lent until the loop continues.
	Chunks(ctx context.Context, text string, size int) iter.Seq2[*native.Buffer, error]
	// Sleep waits for d, or until ctx is canceled.
	Sleep(ctx context.Context, d time.Duration) error
}

// Embedding places words of a language by meaning.
type Embedding interface {
	io.Closer
	// Distance is the distance between two words, 0 for the same.
	Distance(a, b string) float64
	// Neighbors returns the n words nearest word.
	Neighbors(word string, n int) []string
}

// Ranker decides the order of suggestions.
type Ranker interface {
	// Score is higher for better suggestions.
	Score(word string) float64
	// Explain describes a suggestion, which may take a request.
	Explain(ctx context.Context, word string) (string, error)
}

// Unit is what Tokens yields.
type Unit string

const (
	Word      Unit = "word"
	Sentence  Unit = "sentence"
	Paragraph Unit = "paragraph"
)

// Token is a range of the text, in UTF-8 bytes.
type Token struct {
	Start, End int
	Text       string
}

var (
	// ErrUndetermined is returned when the language of a text is unknown.
	ErrUndetermined = native.NewError("undetermined")
	// ErrNoEmbedding is returned for a language without a word embedding.
	ErrNoEmbedding = native.NewError("no_embedding")
)

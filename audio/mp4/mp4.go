// Package mp4 decodes the sound of MP4 files for package audio: .m4a and
// .mp4, whole or cut into fragments as a stream for DASH is (.m4s). It
// reads the container, and a codec registered with RegisterCodec decodes
// its packets. Apple Lossless (ALAC) is built in; import package
// audio/aac for AAC, and audio/flac for FLAC in MP4:
//
//	import (
//		_ "github.com/egoist/mygo/audio/aac"  // AAC, in MP4 too
//		_ "github.com/egoist/mygo/audio/flac" // FLAC, in MP4 too
//	)
//
// Each imports this package, which registers the format with package
// audio.
package mp4

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/egoist/mygo/audio"
)

func init() {
	audio.RegisterFormat("mp4", "????ftyp", func(r io.ReadSeeker) (audio.Decoder, error) { return NewDecoder(r) })
	RegisterCodec("alac", func(config []byte) (Codec, error) { return newALACDecoder(config) })
}

// A Codec decodes the packets of a track, one at a time, to stereo.
type Codec interface {
	// SampleRate returns the frames per second it decodes to.
	SampleRate() int
	// Decode appends the sound of a packet to dst, as interleaved left
	// and right samples in [-1, 1], and returns it. A mono track plays in
	// both; of more channels, the front left and right play.
	Decode(dst []float32, packet []byte) ([]float32, error)
	// Reset forgets the packets decoded before, as after a seek.
	Reset()
	// Preroll returns how many packets before the one holding a frame
	// sought to are decoded first, for codecs whose packets overlap the
	// sound of the one before, as AAC's (1); 0 when packets stand alone.
	Preroll() int
	// PacketFrames returns how many frames every packet decodes to, when
	// all do, as AAC's 1024: packets are then placed in time by counting
	// them, whatever the file says they last. 0 places them as the file
	// says, for codecs whose packets differ.
	PacketFrames() int
}

var (
	codecsMu sync.Mutex
	codecs   = map[string]func([]byte) (Codec, error){}
)

// RegisterCodec registers the codec of tracks whose sample entry is of
// type fourcc, as "mp4a": open returns a decoder of its packets from the
// track's configuration (see Track's Config). A package for a codec calls
// it in its init function.
func RegisterCodec(fourcc string, open func(config []byte) (Codec, error)) {
	codecsMu.Lock()
	defer codecsMu.Unlock()
	codecs[fourcc] = open
}

func openCodec(t *track) (Codec, error) {
	codecsMu.Lock()
	open := codecs[t.Codec]
	codecsMu.Unlock()
	if open == nil {
		return nil, errors.New("mp4: no codec registered for " + t.Codec)
	}
	return open(t.Config)
}

// Track describes an MP4's sound track, as its movie header says, for an
// app to know of a file without decoding it, as a music library scanning
// files does.
type Track struct {
	// Codec is the type of the track's sample entry: "mp4a" (AAC),
	// "alac", "fLaC", "Opus", ...
	Codec string
	// Config configures the codec: AAC's AudioSpecificConfig, ALAC's
	// magic cookie, FLAC's metadata blocks, or another codec's boxes
	// within the sample entry.
	Config []byte
	// Duration is how long it plays, without the encoder's priming, or 0
	// for a fragmented file whose length its header doesn't say.
	Duration time.Duration
	// Fragmented says its packets are in fragments after the movie header.
	Fragmented bool
}

// ReadTrack reads the first sound track of the MP4 file r holds, of size
// bytes, from its movie header.
func ReadTrack(r io.ReaderAt, size int64) (*Track, error) {
	t, err := readTrack(r, size, false)
	if err != nil {
		return nil, err
	}
	return &Track{
		Codec:      t.Codec,
		Config:     t.Config,
		Duration:   time.Duration(max(t.Duration-t.Prime, 0) * int64(time.Second) / t.Timescale),
		Fragmented: t.Fragmented,
	}, nil
}

// Decodable says whether a codec registered decodes the track: HE-AAC, for
// one, package aac does not.
func (t *Track) Decodable() bool {
	_, err := openCodec(&track{Codec: t.Codec, Config: t.Config})
	return err == nil
}

// NewDecoder returns a decoder of the first sound track of the MP4 file r
// holds, which decodes to stereo. It reads the movie header, and the
// packets as they are played, so a file still coming in plays as it comes
// when r waits for the bytes asked of it: a fragmented one, a fragment at
// a time, with its length from its segment index. One with no index is
// read through for its length.
func NewDecoder(r io.ReadSeeker) (audio.Decoder, error) {
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	ra, ok := r.(io.ReaderAt)
	if !ok {
		ra = &readerAt{r: r}
	}
	return newDecoder(ra, size)
}

// readerAt reads at offsets of a ReadSeeker, seeking.
type readerAt struct {
	mu sync.Mutex
	r  io.ReadSeeker
}

func (r *readerAt) ReadAt(b []byte, off int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.r.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	return io.ReadFull(r.r, b)
}

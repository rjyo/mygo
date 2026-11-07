// Package flac decodes FLAC for package audio, with
// github.com/mewkiz/flac: .flac files, and FLAC in MP4 through package
// audio/mp4, as lossless DASH streams carry it. Import it for its side
// effect:
//
//	import _ "github.com/egoist/mygo/audio/flac"
//
// FLAC's frames stand alone, so a seek lands on the very frame of sound
// asked for, without a seek table.
package flac

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"math"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/audio/mp4"
	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

func init() {
	open := func(r io.ReadSeeker) (audio.Decoder, error) { return NewDecoder(r) }
	// After an ID3 tag too, which package audio looks past.
	audio.RegisterFormat("flac", "fLaC", open)
	mp4.RegisterCodec("fLaC", func(config []byte) (mp4.Codec, error) { return NewCodec(config) })
}

// decoder decodes a FLAC file. A seek finds the frame of FLAC holding the
// frame of sound asked for by its header, near where the bytes say it is,
// and decodes from there.
type decoder struct {
	r     io.ReadSeeker
	info  *meta.StreamInfo
	start int64 // the first frame's byte, after the metadata
	size  int64
	br    *bufio.Reader
	skip  int64     // frames to drop, after a seek
	pend  []float32 // decoded, and not yet read: stereo
}

// NewDecoder returns a decoder of the FLAC file r holds, after an ID3 tag
// if one is first. It decodes to stereo: a mono file in both, and the
// front left and right of more channels.
func NewDecoder(r io.ReadSeeker) (audio.Decoder, error) {
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	from, err := skipID3(r)
	if err != nil {
		return nil, err
	}
	start, err := dataStart(r, from)
	if err != nil {
		return nil, err
	}
	head := make([]byte, start-from)
	if _, err := r.Seek(from, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(r, head); err != nil {
		return nil, err
	}
	st, err := flac.New(bytes.NewReader(head))
	if err != nil {
		return nil, err
	}
	if st.Info == nil || st.Info.NChannels == 0 || st.Info.SampleRate == 0 {
		return nil, errors.New("flac: no stream info")
	}
	d := &decoder{r: r, info: st.Info, start: start, size: size}
	if err := d.at(start); err != nil {
		return nil, err
	}
	return d, nil
}

// skipID3 returns where r's FLAC starts: after an ID3 tag, if r starts
// with one.
func skipID3(r io.ReadSeeker) (int64, error) {
	var b [10]byte
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := io.ReadFull(r, b[:]); err != nil || string(b[:3]) != "ID3" {
		return 0, nil
	}
	return 10 + int64(b[6]&0x7f)<<21 | int64(b[7]&0x7f)<<14 | int64(b[8]&0x7f)<<7 | int64(b[9]&0x7f), nil
}

// dataStart returns where the first frame starts, after the "fLaC" at off
// and the metadata blocks.
func dataStart(r io.ReadSeeker, off int64) (int64, error) {
	var h [4]byte
	if _, err := r.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := io.ReadFull(r, h[:]); err != nil || string(h[:]) != "fLaC" {
		return 0, errors.New("flac: not a FLAC file")
	}
	off += 4
	for {
		if _, err := r.Seek(off, io.SeekStart); err != nil {
			return 0, err
		}
		if _, err := io.ReadFull(r, h[:]); err != nil {
			return 0, io.ErrUnexpectedEOF
		}
		off += 4 + (int64(h[1])<<16 | int64(h[2])<<8 | int64(h[3]))
		if h[0]&0x80 != 0 { // the last
			return off, nil
		}
	}
}

// at reads on from off.
func (d *decoder) at(off int64) error {
	if _, err := d.r.Seek(off, io.SeekStart); err != nil {
		return err
	}
	if d.br == nil {
		d.br = bufio.NewReaderSize(d.r, 64<<10)
	} else {
		d.br.Reset(d.r)
	}
	return nil
}

func (d *decoder) SampleRate() int { return int(d.info.SampleRate) }
func (d *decoder) Channels() int   { return 2 }

func (d *decoder) Len() int64 {
	if d.info.NSamples == 0 {
		return -1
	}
	return int64(d.info.NSamples)
}

// parse parses the frame r is at: its header, then its samples, taking
// what the header leaves to the stream info from there. The decoder may
// panic on bytes that only look like a frame.
func parse(r io.Reader, info *meta.StreamInfo) (f *frame.Frame, err error) {
	defer func() {
		if recover() != nil {
			f, err = nil, errors.New("flac: bad frame")
		}
	}()
	f, err = frame.New(r)
	if err != nil {
		return nil, err
	}
	if f.BitsPerSample == 0 {
		f.BitsPerSample = info.BitsPerSample
	}
	if f.SampleRate == 0 {
		f.SampleRate = info.SampleRate
	}
	if err := f.Parse(); err != nil {
		return nil, err
	}
	return f, nil
}

func (d *decoder) Read(dst []float32) (int, error) {
	for len(d.pend) == 0 {
		f, err := parse(d.br, d.info)
		if err != nil {
			if err == io.ErrUnexpectedEOF {
				err = io.EOF
			}
			return 0, err
		}
		drop := min(d.skip, int64(f.BlockSize))
		d.skip -= drop
		d.pend = stereo(d.pend[:0], f, int(drop))
	}
	n := copy(dst[:len(dst)/2*2], d.pend)
	d.pend = d.pend[n:]
	return n, nil
}

// stereo appends a frame's samples from the one at from to dst, as stereo.
func stereo(dst []float32, f *frame.Frame, from int) []float32 {
	left, right := f.Subframes[0].Samples, f.Subframes[0].Samples
	if len(f.Subframes) > 1 {
		right = f.Subframes[1].Samples
	}
	scale := float32(1 / math.Exp2(float64(f.BitsPerSample-1)))
	for i := from; i < int(f.BlockSize); i++ {
		dst = append(dst, float32(left[i])*scale, float32(right[i])*scale)
	}
	return dst
}

// SetPosition goes to target: to the frame of FLAC holding it, then drops
// what comes before it there. It finds that frame by the secant method,
// guessing where in the bytes it is from the frames found on either side,
// until one is found less than a second before it.
func (d *decoder) SetPosition(target int64) error {
	target = max(target, 0)
	total := int64(d.info.NSamples)
	if total > 0 {
		target = min(target, total)
	}
	loOff, lo := d.start, int64(0)
	hiOff, hi := d.size, total
	for tries := 0; tries < 8 && target-lo > int64(d.info.SampleRate) && hi > lo; tries++ {
		guess := loOff + int64(float64(hiOff-loOff)*float64(target-lo)/float64(hi-lo))
		off, at, ok := d.frameAfter(max(guess, loOff+1), hiOff)
		if !ok {
			break
		}
		if at > target {
			hiOff, hi = off, at
		} else {
			loOff, lo = off, at
		}
	}
	d.skip, d.pend = target-lo, d.pend[:0]
	return d.at(loOff)
}

// frameAfter returns where the first whole frame at or after off and before
// end starts, and the frame of sound it starts with. Bytes that only look
// like a frame's header within the audio fail its checksums.
func (d *decoder) frameAfter(off, end int64) (int64, int64, bool) {
	const window = 16 << 10
	buf := make([]byte, window)
	for off+2 < end {
		if _, err := d.r.Seek(off, io.SeekStart); err != nil {
			return 0, 0, false
		}
		n, _ := io.ReadFull(d.r, buf[:min(window, end-off)])
		if n < 2 {
			return 0, 0, false
		}
		for i := 0; i+1 < n; i++ {
			if buf[i] != 0xff || buf[i+1]&0xfe != 0xf8 {
				continue
			}
			if at, ok := d.frameAt(off + int64(i)); ok {
				return off + int64(i), at, true
			}
		}
		off += int64(n - 1)
	}
	return 0, 0, false
}

// frameAt returns the frame of sound the frame of FLAC at off starts with,
// if a whole one is there.
func (d *decoder) frameAt(off int64) (int64, bool) {
	if _, err := d.r.Seek(off, io.SeekStart); err != nil {
		return 0, false
	}
	f, err := parse(bufio.NewReaderSize(d.r, 16<<10), d.info)
	if err != nil || f.Channels.Count() != int(d.info.NChannels) {
		return 0, false
	}
	return int64(f.SampleNumber()), true
}

// codec decodes FLAC frames, each an MP4's packet.
type codec struct {
	info *meta.StreamInfo
	r    bytes.Reader
}

// NewCodec returns a decoder of the FLAC frames of an MP4's track whose
// configuration, its dfLa box's, is blocks: the metadata blocks, as a FLAC
// file has them after "fLaC".
func NewCodec(blocks []byte) (mp4.Codec, error) {
	st, err := flac.New(bytes.NewReader(append([]byte("fLaC"), blocks...)))
	if err != nil {
		return nil, err
	}
	if st.Info == nil || st.Info.SampleRate == 0 || st.Info.NChannels == 0 {
		return nil, errors.New("flac: no stream info")
	}
	return &codec{info: st.Info}, nil
}

func (c *codec) SampleRate() int   { return int(c.info.SampleRate) }
func (c *codec) Reset()            {}
func (c *codec) Preroll() int      { return 0 }
func (c *codec) PacketFrames() int { return 0 }

func (c *codec) Decode(dst []float32, packet []byte) ([]float32, error) {
	c.r.Reset(packet)
	f, err := parse(&c.r, c.info)
	if err != nil {
		return dst, err
	}
	return stereo(dst, f, 0), nil
}

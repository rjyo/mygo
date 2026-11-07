// Package aac decodes AAC-LC for package audio, with
// github.com/tphakala/go-aac: in MP4 (.m4a, and fragmented .m4s as DASH
// streams it), through package audio/mp4, and in ADTS (.aac). Import it
// for its side effect:
//
//	import _ "github.com/egoist/mygo/audio/aac"
//
// HE-AAC (AAC+, with SBR) and HE-AAC v2 it does not decode: their files
// fail to open, and mp4.Track's Decodable says so of them.
//
// go-aac is under the LGPL 2.1 or later, unlike MyGo, whose license is
// MIT: an app that imports this package links it, and takes on the LGPL's
// terms for it.
package aac

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/audio/mp4"
	aacpcm "github.com/tphakala/go-aac/pcm"
)

func init() {
	mp4.RegisterCodec("mp4a", func(config []byte) (mp4.Codec, error) { return NewCodec(config) })
	open := func(r io.ReadSeeker) (audio.Decoder, error) { return NewADTSDecoder(r) }
	// The sync word of an ADTS header, of MPEG-4 or MPEG-2, with a CRC or
	// without; MP3's differ in their layer.
	for _, b := range []string{"\xff\xf1", "\xff\xf0", "\xff\xf9", "\xff\xf8"} {
		audio.RegisterFormat("aac", b, open)
	}
}

// FrameLength is how many frames each AAC packet decodes to.
const FrameLength = 1024

// codec decodes AAC packets, raw as in an MP4 or each with its ADTS header.
type codec struct {
	fd  *aacpcm.FrameDecoder
	pcm []byte
}

// NewCodec returns a decoder of the raw AAC packets of a track configured
// by asc, its AudioSpecificConfig: an MP4's, for package mp4.
func NewCodec(asc []byte) (mp4.Codec, error) {
	fd, err := aacpcm.NewRawDecoder(asc)
	if err != nil {
		return nil, err
	}
	return &codec{fd: fd}, nil
}

func (c *codec) SampleRate() int   { return c.fd.SampleRate() }
func (c *codec) Reset()            { c.fd.Reset() }
func (c *codec) Preroll() int      { return 1 }
func (c *codec) PacketFrames() int { return FrameLength }

func (c *codec) Decode(dst []float32, packet []byte) ([]float32, error) {
	pcm, n, err := c.fd.DecodeFrame(c.pcm[:0], packet)
	if err != nil {
		return dst, err
	}
	c.pcm = pcm
	ch := c.fd.Channels()
	right := 0
	if ch > 1 {
		right = 2
	}
	for i := range n {
		f := pcm[i*ch*2:]
		dst = append(dst,
			float32(int16(binary.LittleEndian.Uint16(f)))/(1<<15),
			float32(int16(binary.LittleEndian.Uint16(f[right:])))/(1<<15))
	}
	return dst, nil
}

var adtsRates = [...]int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}

// adtsHeader reads an ADTS frame's header: its length in bytes, and its
// sample rate.
func adtsHeader(h []byte) (length, rate int, ok bool) {
	if len(h) < 7 || h[0] != 0xff || h[1]&0xf6 != 0xf0 {
		return 0, 0, false
	}
	i := int(h[2] >> 2 & 15)
	length = int(h[3]&3)<<11 | int(h[4])<<3 | int(h[5]>>5)
	if i >= len(adtsRates) || length < 7 {
		return 0, 0, false
	}
	return length, adtsRates[i], true
}

// adts decodes AAC in ADTS frames, each with a header saying how long it
// is, which it finds all of as it opens, for the length and to seek.
type adts struct {
	r     io.ReadSeeker
	c     *codec
	rate  int
	offs  []int64
	sizes []int
	next  int   // the packet decoded next
	skip  int64 // frames to drop before the frame Read returns next
	buf   []byte
	out   []float32
	pend  []float32
}

// NewADTSDecoder returns a decoder of the AAC in ADTS frames r holds, as
// an .aac file, after an ID3 tag if one is first. It decodes to stereo.
func NewADTSDecoder(r io.ReadSeeker) (audio.Decoder, error) {
	start, err := skipID3(r)
	if err != nil {
		return nil, err
	}
	d := &adts{r: r, c: &codec{fd: aacpcm.NewADTSDecoder()}}
	var h [7]byte
	for off := start; ; {
		if _, err := r.Seek(off, io.SeekStart); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(r, h[:]); err != nil {
			break
		}
		length, rate, ok := adtsHeader(h[:])
		if !ok {
			break
		}
		if d.rate == 0 {
			d.rate = rate
		}
		d.offs = append(d.offs, off)
		d.sizes = append(d.sizes, length)
		off += int64(length)
	}
	if len(d.offs) == 0 {
		return nil, errors.New("aac: not an ADTS stream")
	}
	return d, d.SetPosition(0)
}

// skipID3 returns where r's audio starts: after an ID3 tag, if r starts
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

func (d *adts) SampleRate() int { return d.rate }
func (d *adts) Channels() int   { return 2 }
func (d *adts) Len() int64      { return int64(len(d.offs)) * FrameLength }

func (d *adts) Read(dst []float32) (int, error) {
	for len(d.pend) == 0 {
		if d.next >= len(d.offs) {
			return 0, io.EOF
		}
		size := d.sizes[d.next]
		if cap(d.buf) < size {
			d.buf = make([]byte, size)
		}
		d.buf = d.buf[:size]
		if _, err := d.r.Seek(d.offs[d.next], io.SeekStart); err != nil {
			return 0, err
		}
		if _, err := io.ReadFull(d.r, d.buf); err != nil {
			return 0, io.EOF
		}
		d.next++
		out, err := d.c.Decode(d.out[:0], d.buf)
		d.out = out
		if err != nil {
			continue // a packet gone bad: on to the next
		}
		drop := min(d.skip, int64(len(out)/2))
		d.skip -= drop
		d.pend = out[drop*2:]
	}
	n := copy(dst[:len(dst)/2*2], d.pend)
	d.pend = d.pend[n:]
	return n, nil
}

// SetPosition goes to the packet holding frame, from the one before, whose
// sound overlaps it, and drops what comes before frame.
func (d *adts) SetPosition(frame int64) error {
	frame = min(max(frame, 0), d.Len())
	i := int(frame / FrameLength)
	start := max(0, i-1)
	d.c.Reset()
	d.next = start
	d.skip = frame - int64(start)*FrameLength
	d.pend = nil
	return nil
}

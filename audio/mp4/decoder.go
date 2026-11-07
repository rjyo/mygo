package mp4

import (
	"encoding/binary"
	"errors"
	"io"
	"sort"
)

// A decoder decodes a track's packets, whose places it knows from the
// movie header's sample table or, of a fragmented file, from each
// fragment's header as it reaches it. A seek goes to the packet holding the
// frame asked for.
type decoder struct {
	r     io.ReaderAt
	codec Codec
	rate  int
	offs  []int64
	sizes []int
	ends  []int64 // where each packet ends, in frames
	// last is where the last packet found ends, in frames, as the file
	// says, when ends places packets by the codec's PacketFrames instead;
	// 0 when ends is as the file says.
	last int64
	// more finds the next fragment's packets, and says if there were
	// any. Nil once all are found.
	more   func() bool
	prime  int64 // the encoder's priming, in frames, to drop
	frames int64 // the length, or -1 if unknown
	exact  bool  // whether it is: then what's past it is padding, dropped
	next   int   // the packet decoded next
	pos    int64 // the frame Read returns next
	skip   int64 // frames to drop before it
	packet []byte
	out    []float32
	pend   []float32
}

func newDecoder(r io.ReaderAt, size int64) (*decoder, error) {
	t, err := readTrack(r, size, true)
	if err != nil {
		return nil, err
	}
	c, err := openCodec(t)
	if err != nil {
		return nil, err
	}
	d := &decoder{r: r, codec: c, rate: c.SampleRate(), frames: -1}
	if d.rate <= 0 {
		return nil, errors.New("mp4: no sample rate")
	}
	// In frames, from the track's timescale: most often the same.
	frames := func(v int64) int64 { return v * int64(d.rate) / t.Timescale }
	d.prime = frames(t.Prime)
	d.skip = d.prime
	if !t.Fragmented {
		for i := range t.Sizes {
			d.add(t.Offsets[i], int(t.Sizes[i]), frames(t.Ends[i]))
		}
		if len(d.ends) == 0 {
			return nil, errors.New("mp4: no packets")
		}
		d.frames, d.exact = d.length()-d.prime, true
		return d, nil
	}
	d.fragments(t, size, frames)
	if t.Duration > 0 {
		// From the segment index: the fragments are found as they play.
		d.frames = frames(t.Duration) - d.prime
	} else {
		for d.findMore() {
		}
	}
	if !d.have(0) {
		return nil, errors.New("mp4: no fragments")
	}
	return d, nil
}

func (d *decoder) SampleRate() int { return d.rate }
func (d *decoder) Channels() int   { return 2 }
func (d *decoder) Len() int64      { return d.frames }

// add adds a packet of size bytes at off, ending at end frames as the file
// says.
func (d *decoder) add(off int64, size int, end int64) {
	d.offs = append(d.offs, off)
	d.sizes = append(d.sizes, size)
	if n := d.codec.PacketFrames(); n > 0 {
		d.last = end
		end = int64(len(d.ends)+1) * int64(n)
	}
	d.ends = append(d.ends, end)
}

// length is where the packets found end, in frames, as the file says.
func (d *decoder) length() int64 {
	if d.last > 0 {
		return d.last
	}
	return d.ends[len(d.ends)-1]
}

// packetStart is the frame packet i starts at.
func (d *decoder) packetStart(i int) int64 {
	if i == 0 {
		return 0
	}
	return d.ends[i-1]
}

// have says whether packet i is known, finding more if need be.
func (d *decoder) have(i int) bool {
	for i >= len(d.offs) {
		if !d.findMore() {
			return false
		}
	}
	return true
}

// findMore finds the next fragment's packets, if there are any left to
// find. Once all are found, the length is known.
func (d *decoder) findMore() bool {
	if d.more == nil {
		return false
	}
	if d.more() {
		return true
	}
	d.more = nil
	if len(d.ends) > 0 {
		d.frames, d.exact = d.length()-d.prime, true
	}
	return false
}

func (d *decoder) Read(dst []float32) (int, error) {
	for len(d.pend) == 0 {
		if !d.have(d.next) {
			return 0, io.EOF
		}
		if !d.exact {
			// Whether this is the last packet, whose padding is dropped:
			// the next fragment's are found, if any.
			d.have(d.next + 1)
		}
		size := d.sizes[d.next]
		if cap(d.packet) < size {
			d.packet = make([]byte, size)
		}
		d.packet = d.packet[:size]
		if _, err := d.r.ReadAt(d.packet, d.offs[d.next]); err != nil {
			if err == io.ErrUnexpectedEOF {
				err = io.EOF // cut short
			}
			return 0, err
		}
		d.next++
		out, err := d.codec.Decode(d.out[:0], d.packet)
		d.out = out
		if err != nil {
			continue // a packet gone bad: on to the next
		}
		drop := min(d.skip, int64(len(out)/2))
		d.skip -= drop
		d.pend = out[drop*2:]
	}
	n := copy(dst[:len(dst)/2*2], d.pend)
	if d.exact {
		// What the encoder padded the last packet with.
		n = int(min(int64(n), (d.frames-d.pos)*2))
		if n <= 0 {
			d.pend = nil
			return 0, io.EOF
		}
	}
	d.pend = d.pend[n:]
	d.pos += int64(n / 2)
	return n, nil
}

// SetPosition goes to the packet holding frame, from those before it its
// sound overlaps, and drops what comes before frame.
func (d *decoder) SetPosition(frame int64) error {
	frame = max(frame, 0)
	if d.frames >= 0 {
		frame = min(frame, d.frames)
	}
	at := frame + d.prime
	for (len(d.ends) == 0 || d.ends[len(d.ends)-1] <= at) && d.findMore() {
	}
	i := sort.Search(len(d.ends), func(i int) bool { return d.ends[i] > at })
	start := max(0, i-d.codec.Preroll())
	d.codec.Reset()
	d.next, d.pos = start, frame
	d.skip = at - d.packetStart(start)
	d.pend = nil
	return nil
}

// fragments has the decoder find a fragmented file's packets as it needs
// them, a fragment at a time, walking the boxes after the movie header.
func (d *decoder) fragments(t *track, size int64, frames func(int64) int64) {
	off := t.Next
	var h [16]byte
	d.more = func() bool {
		for off+8 <= size {
			if _, err := d.r.ReadAt(h[:8], off); err != nil {
				return false
			}
			n, head := int64(binary.BigEndian.Uint32(h[:])), int64(8)
			if n == 1 {
				if _, err := d.r.ReadAt(h[8:], off+8); err != nil {
					return false
				}
				n, head = int64(binary.BigEndian.Uint64(h[8:])), 16
			}
			if n < head {
				return false // to the end, or broken: no more fragments
			}
			at := off
			off += n
			if string(h[4:8]) != "moof" {
				continue
			}
			if n > 16<<20 {
				return false
			}
			moof := make([]byte, n-head)
			if _, err := d.r.ReadAt(moof, at+head); err != nil {
				return false
			}
			found := len(t.Sizes)
			if err := t.fragment(moof, at); err != nil {
				return false
			}
			for i := found; i < len(t.Sizes); i++ {
				d.add(t.Offsets[i], int(t.Sizes[i]), frames(t.Ends[i]))
			}
			if len(t.Sizes) > found {
				return true
			}
		}
		return false
	}
}

package mp4

import (
	"encoding/binary"
	"errors"
	"io"
	"iter"
)

// A track is an MP4's first sound track.
type track struct {
	// Codec is the type of its sample entry: "mp4a", "alac", "fLaC", ...
	Codec string
	// Config is AAC's AudioSpecificConfig, ALAC's magic cookie, FLAC's
	// metadata blocks, or another codec's boxes within the sample entry.
	Config    []byte
	Timescale int64 // the units of its times, a second's
	// Duration is how long it is, in the timescale, as its header says.
	Duration int64
	// Prime is where the edit list starts it: the encoder's priming, in
	// the timescale, to drop.
	Prime int64
	// Where each packet is, and where it ends, in the timescale. Only
	// read with the tables.
	Offsets []int64
	Sizes   []uint32
	Ends    []int64

	// Fragmented says the packets aren't in the movie header's tables but
	// in fragments after it, as a stream cut up for DASH has them: the
	// tables fill in as fragment reads each. Next is where the boxes after
	// the movie header start.
	Fragmented bool
	Next       int64
	id         uint32 // the track's ID, which its fragments name
	defaults   fragmentDefaults
}

// fragmentDefaults are what a fragment's packets are unless it says.
type fragmentDefaults struct {
	duration, size uint32
}

// ErrNoMovie says an MP4 has no movie header: no tracks to play.
var ErrNoMovie = errors.New("mp4: no movie header")

// readTrack reads the sound track of the MP4 r holds, of size bytes. With tables,
// it reads where each packet is too. The movie header is often after the
// media data: r may wait for the bytes asked of it, as a file coming in.
func readTrack(r io.ReaderAt, size int64, tables bool) (*track, error) {
	var h [16]byte
	for off := int64(0); off+8 <= size; {
		if _, err := r.ReadAt(h[:8], off); err != nil {
			return nil, ErrNoMovie
		}
		n, head := int64(binary.BigEndian.Uint32(h[:])), int64(8)
		switch n {
		case 0: // to the end
			n = size - off
		case 1:
			if _, err := r.ReadAt(h[8:], off+8); err != nil {
				return nil, ErrNoMovie
			}
			n, head = int64(binary.BigEndian.Uint64(h[8:])), 16
		}
		if n < head || n > size-off {
			return nil, errors.New("mp4: bad box")
		}
		if string(h[4:8]) == "moov" {
			if n > 256<<20 {
				return nil, errors.New("mp4: movie header too big")
			}
			moov := make([]byte, n-head)
			if _, err := r.ReadAt(moov, off+head); err != nil {
				return nil, err
			}
			a, err := parseMoov(moov, tables)
			if err != nil || !a.Fragmented {
				return a, err
			}
			a.Next = off + n
			a.readIndex(r, size)
			return a, nil
		}
		off += n
	}
	return nil, ErrNoMovie
}

// boxes walks the boxes in b, giving each one's type and body.
func boxes(b []byte) iter.Seq2[string, []byte] {
	return func(yield func(string, []byte) bool) {
		for len(b) >= 8 {
			size, head := uint64(binary.BigEndian.Uint32(b)), uint64(8)
			typ := string(b[4:8])
			switch size {
			case 0:
				size = uint64(len(b))
			case 1:
				if len(b) < 16 {
					return
				}
				size, head = binary.BigEndian.Uint64(b[8:]), 16
			}
			if size < head || size > uint64(len(b)) {
				return
			}
			if !yield(typ, b[head:size]) {
				return
			}
			b = b[size:]
		}
	}
}

// child returns the body of the box of type typ in b, following the path of
// types after it into the boxes within.
func child(b []byte, path ...string) []byte {
	for _, typ := range path {
		var found []byte
		for t, body := range boxes(b) {
			if t == typ {
				found = body
				break
			}
		}
		if found == nil {
			return nil
		}
		b = found
	}
	return b
}

// parseMoov finds the first sound track in a movie header.
func parseMoov(moov []byte, tables bool) (*track, error) {
	mvex := child(moov, "mvex")
	for typ, trak := range boxes(moov) {
		if typ != "trak" {
			continue
		}
		mdia := child(trak, "mdia")
		if hdlr := child(mdia, "hdlr"); len(hdlr) < 12 || string(hdlr[8:12]) != "soun" {
			continue
		}
		a, err := parseTrack(trak, mdia, mvex, tables)
		if err != nil {
			return nil, err
		}
		return a, nil
	}
	return nil, errors.New("mp4: no sound track")
}

var errMP4Table = errors.New("mp4: bad sample table")

func parseTrack(trak, mdia, mvex []byte, tables bool) (*track, error) {
	a := &track{}
	mdhd := child(mdia, "mdhd")
	switch {
	case len(mdhd) >= 32 && mdhd[0] == 1:
		a.Timescale = int64(binary.BigEndian.Uint32(mdhd[20:]))
		a.Duration = int64(binary.BigEndian.Uint64(mdhd[24:]))
	case len(mdhd) >= 20:
		a.Timescale = int64(binary.BigEndian.Uint32(mdhd[12:]))
		a.Duration = int64(binary.BigEndian.Uint32(mdhd[16:]))
	}
	if a.Timescale <= 0 || a.Duration < 0 {
		return nil, errors.New("mp4: no timescale")
	}
	stbl := child(mdia, "minf", "stbl")
	if err := a.parseEntry(child(stbl, "stsd")); err != nil {
		return nil, err
	}
	if mvex != nil && !hasSamples(stbl) {
		a.Fragmented = true
		if tkhd := child(trak, "tkhd"); len(tkhd) >= 16 {
			at := 12
			if tkhd[0] == 1 {
				at = 20
			}
			a.id = binary.BigEndian.Uint32(tkhd[at:])
		}
		for typ, trex := range boxes(mvex) {
			if typ == "trex" && len(trex) >= 24 && binary.BigEndian.Uint32(trex[4:]) == a.id {
				a.defaults = fragmentDefaults{binary.BigEndian.Uint32(trex[12:]), binary.BigEndian.Uint32(trex[16:])}
			}
		}
	} else if tables {
		if err := a.parseTables(stbl); err != nil {
			return nil, err
		}
		a.Duration = a.Ends[len(a.Ends)-1]
	}
	if elst := child(trak, "edts", "elst"); len(elst) >= 8 {
		n := binary.BigEndian.Uint32(elst[4:])
		b := elst[8:]
		for range n {
			var t int64
			if elst[0] == 1 {
				if len(b) < 20 {
					break
				}
				t, b = int64(binary.BigEndian.Uint64(b[8:])), b[20:]
			} else {
				if len(b) < 12 {
					break
				}
				t, b = int64(int32(binary.BigEndian.Uint32(b[4:]))), b[12:]
			}
			if t >= 0 { // past an empty edit, which plays nothing
				a.Prime = t
				if a.Duration > 0 {
					a.Prime = min(t, a.Duration)
				}
				break
			}
		}
	}
	return a, nil
}

// parseEntry reads the track's sample description: its codec and its
// configuration, in a box within the entry, or within a 'wave' box there in
// QuickTime's own kind.
func (a *track) parseEntry(stsd []byte) error {
	if len(stsd) < 8+36 {
		return errors.New("mp4: no sample description")
	}
	entry := stsd[8:]
	size := int(binary.BigEndian.Uint32(entry))
	if size < 36 || size > len(entry) {
		return errors.New("mp4: bad sample description")
	}
	a.Codec = string(entry[4:8])
	inner := entry[36:size]
	switch binary.BigEndian.Uint16(entry[16:]) { // the version of the sound description
	case 1:
		inner = inner[min(16, len(inner)):]
	case 2:
		inner = inner[min(36, len(inner)):]
	}
	if wave := child(inner, "wave"); wave != nil {
		inner = wave
	}
	switch a.Codec {
	case "alac":
		cookie := child(inner, "alac")
		if len(cookie) < 4 {
			return errors.New("mp4: no ALAC configuration")
		}
		a.Config = cookie[4:]
	case "fLaC":
		dfla := child(inner, "dfLa")
		if len(dfla) < 4+4+34 {
			return errors.New("mp4: no FLAC configuration")
		}
		a.Config = dfla[4:]
	case "mp4a":
		esds := child(inner, "esds")
		if len(esds) < 4 {
			return errors.New("mp4: no AAC configuration")
		}
		asc, ok := decoderConfig(esds[4:])
		if !ok {
			return errors.New("mp4: not AAC")
		}
		a.Config = asc
	default:
		a.Config = inner
	}
	return nil
}

// decoderConfig finds the DecoderSpecificInfo in an elementary stream's
// descriptors: AAC's AudioSpecificConfig. A stream not of AAC has none it
// takes, as an MP3's in an MP4.
func decoderConfig(b []byte) ([]byte, bool) {
	descriptor := func(b []byte) (tag byte, body, rest []byte, ok bool) {
		if len(b) < 2 {
			return 0, nil, nil, false
		}
		tag, b = b[0], b[1:]
		n := 0
		for i := 0; i < 4 && len(b) > 0; i++ {
			c := b[0]
			b = b[1:]
			n = n<<7 | int(c&0x7f)
			if c&0x80 == 0 {
				break
			}
		}
		if n > len(b) {
			return 0, nil, nil, false
		}
		return tag, b[:n], b[n:], true
	}
	tag, es, _, ok := descriptor(b)
	if !ok || tag != 3 || len(es) < 3 {
		return nil, false
	}
	flags := es[2]
	es = es[3:]
	if flags&0x80 != 0 { // depends on a stream
		es = es[min(2, len(es)):]
	}
	if flags&0x40 != 0 && len(es) > 0 { // a URL
		es = es[min(1+int(es[0]), len(es)):]
	}
	if flags&0x20 != 0 { // an OCR stream
		es = es[min(2, len(es)):]
	}
	tag, dc, _, ok := descriptor(es)
	if !ok || tag != 4 || len(dc) < 13 {
		return nil, false
	}
	switch dc[0] { // MPEG-4 audio, or MPEG-2 AAC's profiles
	case 0x40, 0x66, 0x67, 0x68:
	default:
		return nil, false
	}
	tag, asc, _, ok := descriptor(dc[13:])
	if !ok || tag != 5 || len(asc) < 2 {
		return nil, false
	}
	return asc, true
}

// maxSamples bounds the tables read, against a file that claims more than it
// could hold: a day of AAC at 48 kHz.
const maxSamples = 1 << 22

// parseTables reads the sample table: each packet's size, which chunk holds
// it, where that chunk is, and how long each packet lasts.
func (a *track) parseTables(stbl []byte) error {
	// Sizes.
	if stsz := child(stbl, "stsz"); len(stsz) >= 12 {
		fixed := binary.BigEndian.Uint32(stsz[4:])
		n := int(binary.BigEndian.Uint32(stsz[8:]))
		if n > maxSamples || fixed == 0 && len(stsz) < 12+4*n {
			return errMP4Table
		}
		a.Sizes = make([]uint32, n)
		for i := range a.Sizes {
			if fixed != 0 {
				a.Sizes[i] = fixed
			} else {
				a.Sizes[i] = binary.BigEndian.Uint32(stsz[12+4*i:])
			}
		}
	} else if stz2 := child(stbl, "stz2"); len(stz2) >= 12 {
		field := int(stz2[7])
		n := int(binary.BigEndian.Uint32(stz2[8:]))
		if n > maxSamples || field != 4 && field != 8 && field != 16 || len(stz2) < 12+(n*field+7)/8 {
			return errMP4Table
		}
		a.Sizes = make([]uint32, n)
		b := stz2[12:]
		for i := range a.Sizes {
			switch field {
			case 4:
				a.Sizes[i] = uint32(b[i/2]>>(4*(1-i%2))) & 15
			case 8:
				a.Sizes[i] = uint32(b[i])
			case 16:
				a.Sizes[i] = uint32(binary.BigEndian.Uint16(b[2*i:]))
			}
		}
	} else {
		return errMP4Table
	}
	if len(a.Sizes) == 0 {
		return errMP4Table
	}

	// Where the chunks are.
	var chunks []int64
	if stco := child(stbl, "stco"); len(stco) >= 8 {
		n := int(binary.BigEndian.Uint32(stco[4:]))
		if n > maxSamples || len(stco) < 8+4*n {
			return errMP4Table
		}
		chunks = make([]int64, n)
		for i := range chunks {
			chunks[i] = int64(binary.BigEndian.Uint32(stco[8+4*i:]))
		}
	} else if co64 := child(stbl, "co64"); len(co64) >= 8 {
		n := int(binary.BigEndian.Uint32(co64[4:]))
		if n > maxSamples || len(co64) < 8+8*n {
			return errMP4Table
		}
		chunks = make([]int64, n)
		for i := range chunks {
			chunks[i] = int64(binary.BigEndian.Uint64(co64[8+8*i:]))
		}
	} else {
		return errMP4Table
	}

	// Which packets each chunk holds, in runs of chunks alike.
	stsc := child(stbl, "stsc")
	if len(stsc) < 8 {
		return errMP4Table
	}
	runs := int(binary.BigEndian.Uint32(stsc[4:]))
	if runs > maxSamples || len(stsc) < 8+12*runs || runs == 0 {
		return errMP4Table
	}
	a.Offsets = make([]int64, len(a.Sizes))
	s := 0
	for r := 0; r < runs && s < len(a.Sizes); r++ {
		e := stsc[8+12*r:]
		first := int(binary.BigEndian.Uint32(e)) - 1
		per := int(binary.BigEndian.Uint32(e[4:]))
		last := len(chunks)
		if r+1 < runs {
			last = int(binary.BigEndian.Uint32(stsc[8+12*(r+1):])) - 1
		}
		if first < 0 || last > len(chunks) || per <= 0 {
			return errMP4Table
		}
		for c := first; c < last && s < len(a.Sizes); c++ {
			off := chunks[c]
			for range per {
				if s == len(a.Sizes) {
					break
				}
				a.Offsets[s] = off
				off += int64(a.Sizes[s])
				s++
			}
		}
	}
	if s < len(a.Sizes) {
		a.Sizes, a.Offsets = a.Sizes[:s], a.Offsets[:s]
	}

	// How long each lasts.
	stts := child(stbl, "stts")
	if len(stts) < 8 {
		return errMP4Table
	}
	n := int(binary.BigEndian.Uint32(stts[4:]))
	if len(stts) < 8+8*n {
		return errMP4Table
	}
	a.Ends = make([]int64, 0, len(a.Sizes))
	var t int64
	for i := 0; i < n && len(a.Ends) < len(a.Sizes); i++ {
		count := int(binary.BigEndian.Uint32(stts[8+8*i:]))
		delta := int64(binary.BigEndian.Uint32(stts[12+8*i:]))
		for range min(count, len(a.Sizes)-len(a.Ends)) {
			t += delta
			a.Ends = append(a.Ends, t)
		}
	}
	if len(a.Ends) == 0 {
		return errMP4Table
	}
	a.Sizes, a.Offsets = a.Sizes[:len(a.Ends)], a.Offsets[:len(a.Ends)]
	return nil
}

// hasSamples says whether a sample table lists any packets: a fragmented
// file's lists none, its packets being in the fragments.
func hasSamples(stbl []byte) bool {
	if stsz := child(stbl, "stsz"); len(stsz) >= 12 {
		return binary.BigEndian.Uint32(stsz[8:]) > 0
	}
	if stz2 := child(stbl, "stz2"); len(stz2) >= 12 {
		return binary.BigEndian.Uint32(stz2[8:]) > 0
	}
	return false
}

// readIndex reads how long a fragmented track is from the segment index
// that comes before its first fragment, if there's one: the movie header
// of such a file says nothing of it.
func (a *track) readIndex(r io.ReaderAt, size int64) {
	var h [8]byte
	for off := a.Next; off+8 <= size; {
		if _, err := r.ReadAt(h[:], off); err != nil {
			return
		}
		n := int64(binary.BigEndian.Uint32(h[:]))
		switch string(h[4:]) {
		case "sidx":
			if n < 8+24 || n > 1<<20 || n > size-off {
				return
			}
			sidx := make([]byte, n-8)
			if _, err := r.ReadAt(sidx, off+8); err != nil {
				return
			}
			if d, ok := indexDuration(sidx); ok && a.Timescale > 0 {
				a.Duration = d * a.Timescale / int64(binary.BigEndian.Uint32(sidx[8:]))
			}
			return
		case "moof", "mdat":
			return
		}
		if n < 8 {
			return
		}
		off += n
	}
}

// indexDuration adds up the durations of the segments a segment index lists.
func indexDuration(sidx []byte) (int64, bool) {
	at := 20
	if len(sidx) > 0 && sidx[0] == 1 {
		at = 28
	}
	if len(sidx) < at+4 || binary.BigEndian.Uint32(sidx[8:]) == 0 {
		return 0, false
	}
	b := sidx[at:]
	n := int(binary.BigEndian.Uint16(b[2:]))
	b = b[4:]
	if len(b) < 12*n {
		return 0, false
	}
	var d int64
	for i := range n {
		d += int64(binary.BigEndian.Uint32(b[12*i+4:]))
	}
	return d, d > 0
}

// fragment reads a fragment's movie header, the body of the moof box at off
// in the file, adding where the track's packets are in it to the tables.
func (a *track) fragment(moof []byte, off int64) error {
	for typ, traf := range boxes(moof) {
		if typ != "traf" {
			continue
		}
		tfhd := child(traf, "tfhd")
		if len(tfhd) < 8 || binary.BigEndian.Uint32(tfhd[4:]) != a.id {
			continue
		}
		flags := binary.BigEndian.Uint32(tfhd) & 0xffffff
		base := off // where the moof starts, unless it says
		d := a.defaults
		b := tfhd[8:]
		field := func(flag uint32, n int) ([]byte, bool) {
			if flags&flag == 0 {
				return nil, false
			}
			if len(b) < n {
				return nil, false
			}
			v := b[:n]
			b = b[n:]
			return v, true
		}
		if v, ok := field(0x01, 8); ok {
			base = int64(binary.BigEndian.Uint64(v))
		}
		field(0x02, 4) // the sample description: there's one
		if v, ok := field(0x08, 4); ok {
			d.duration = binary.BigEndian.Uint32(v)
		}
		if v, ok := field(0x10, 4); ok {
			d.size = binary.BigEndian.Uint32(v)
		}
		at := base
		for typ, trun := range boxes(traf) {
			if typ != "trun" {
				continue
			}
			var err error
			if at, err = a.run(trun, base, at, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// run reads a track run: packets one after another, from at unless it says
// where from base. It returns where the next run starts, unless it says.
func (a *track) run(trun []byte, base, at int64, d fragmentDefaults) (int64, error) {
	if len(trun) < 8 {
		return 0, errMP4Table
	}
	flags := binary.BigEndian.Uint32(trun) & 0xffffff
	n := int(binary.BigEndian.Uint32(trun[4:]))
	b := trun[8:]
	if flags&0x01 != 0 {
		if len(b) < 4 {
			return 0, errMP4Table
		}
		at = base + int64(int32(binary.BigEndian.Uint32(b)))
		b = b[4:]
	}
	if flags&0x04 != 0 { // the first packet's flags
		if len(b) < 4 {
			return 0, errMP4Table
		}
		b = b[4:]
	}
	per := 0
	for _, f := range []uint32{0x100, 0x200, 0x400, 0x800} {
		if flags&f != 0 {
			per += 4
		}
	}
	if n > maxSamples || len(b) < n*per || len(a.Sizes)+n > maxSamples {
		return 0, errMP4Table
	}
	var t int64
	if len(a.Ends) > 0 {
		t = a.Ends[len(a.Ends)-1]
	}
	for range n {
		duration, size := d.duration, d.size
		e := b
		if flags&0x100 != 0 {
			duration, e = binary.BigEndian.Uint32(e), e[4:]
		}
		if flags&0x200 != 0 {
			size = binary.BigEndian.Uint32(e)
		}
		b = b[per:]
		t += int64(duration)
		a.Offsets = append(a.Offsets, at)
		a.Sizes = append(a.Sizes, size)
		a.Ends = append(a.Ends, t)
		at += int64(size)
	}
	return at, nil
}

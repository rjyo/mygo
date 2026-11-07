package mp4

import (
	"encoding/binary"
	"errors"
	"math/bits"
)

// An ALAC decoder decodes Apple Lossless packets, as an MP4 holds them, after
// Apple's own decoder (https://github.com/macosforge/alac, under the Apache
// License 2.0): each packet a frame of up to frameLength
// samples a channel, adaptively Golomb coded, then predicted from the samples
// before it. It gives stereo, of a mono file in both ears.
type alacDecoder struct {
	frameLength     uint32
	bitDepth        uint8
	pb, mb, kb      uint8
	channels        uint8
	rate            uint32
	predictor       []int32
	u, v            []int32 // a channel pair's, mixed
	shift           []uint16
	left, right     []int32 // the channels kept: the front pair
	leftCh, rightCh int
	bitsPerSample   float32
	coefsU, coefsV  [32]int16
	packet          []byte // the last, with zeros after it
}

// ALAC elements, as AAC's.
const (
	alacSCE = 0 // a single channel
	alacCPE = 1 // a channel pair
	alacCCE = 2
	alacLFE = 3
	alacDSE = 4
	alacPCE = 5
	alacFIL = 6
	alacEND = 7
)

// newALACDecoder reads the stream's magic cookie: its ALACSpecificConfig, in
// an MP4 within an 'alac' atom of its own, and in QuickTime's after a 'frma'.
func newALACDecoder(cookie []byte) (*alacDecoder, error) {
	for len(cookie) >= 12 {
		switch string(cookie[4:8]) {
		case "frma":
			cookie = cookie[12:]
			continue
		case "alac":
			cookie = cookie[12:]
			continue
		}
		break
	}
	if len(cookie) < 24 {
		return nil, errors.New("alac: short magic cookie")
	}
	d := &alacDecoder{
		frameLength: binary.BigEndian.Uint32(cookie),
		bitDepth:    cookie[5],
		pb:          cookie[6],
		mb:          cookie[7],
		kb:          cookie[8],
		channels:    cookie[9],
		rate:        binary.BigEndian.Uint32(cookie[20:]),
	}
	switch {
	case cookie[4] != 0:
		return nil, errors.New("alac: unknown version")
	case d.frameLength == 0 || d.frameLength > 1<<16:
		return nil, errors.New("alac: bad frame length")
	case d.bitDepth != 16 && d.bitDepth != 20 && d.bitDepth != 24 && d.bitDepth != 32:
		return nil, errors.New("alac: bad bit depth")
	case d.channels == 0 || d.channels > 8:
		return nil, errors.New("alac: bad channel count")
	case d.kb == 0 || d.kb > 30:
		return nil, errors.New("alac: bad parameters")
	}
	n := int(d.frameLength)
	d.predictor = make([]int32, n)
	d.u, d.v = make([]int32, n), make([]int32, n)
	d.shift = make([]uint16, 2*n)
	d.left, d.right = make([]int32, n), make([]int32, n)
	// The front left and right, of ALAC's channel layouts: C L R first from
	// three channels, and C Lc Rc L R from eight.
	switch d.channels {
	case 1:
		d.leftCh, d.rightCh = 0, 0
	case 2:
		d.leftCh, d.rightCh = 0, 1
	case 8:
		d.leftCh, d.rightCh = 3, 4
	default:
		d.leftCh, d.rightCh = 1, 2
	}
	d.bitsPerSample = 1 / float32(int64(1)<<(d.bitDepth-1))
	return d, nil
}

// alacBits reads a packet's bits, most significant first. Reads past its end
// read zeros, and mark it overrun.
type alacBits struct {
	b   []byte // with 8 bytes of zeros after it
	n   uint   // its bits, but the zeros
	pos uint
}

func (r *alacBits) at(pos, n uint) uint32 {
	if n == 0 {
		return 0
	}
	i := pos >> 3
	if i+8 > uint(len(r.b)) {
		return 0
	}
	return uint32(binary.BigEndian.Uint64(r.b[i:]) << (pos & 7) >> (64 - n))
}

func (r *alacBits) read(n uint) uint32 {
	v := r.at(r.pos, n)
	r.pos += n
	return v
}

func (r *alacBits) over() bool { return r.pos > r.n }

var errALACData = errors.New("alac: bad packet")

// Decode decodes a packet into dst, as stereo, and returns it.
func (d *alacDecoder) Decode(dst []float32, packet []byte) ([]float32, error) {
	// With zeros after it, for reads of 8 bytes at a time near its end.
	d.packet = append(append(d.packet[:0], packet...), make([]byte, 8)...)
	r := &alacBits{b: d.packet, n: uint(len(packet)) * 8}
	ch := 0
	samples := -1
	for {
		if r.over() {
			return dst, errALACData
		}
		tag := r.read(3)
		switch tag {
		case alacSCE, alacLFE, alacCPE:
			if ch >= int(d.channels) {
				return dst, errALACData
			}
			pair := tag == alacCPE
			n, err := d.element(r, pair, ch)
			if err != nil {
				return dst, err
			}
			if samples >= 0 && n != samples {
				return dst, errALACData
			}
			samples = n
			if pair {
				ch += 2
			} else {
				ch++
			}
		case alacDSE:
			r.read(4) // its instance
			align := r.read(1)
			count := r.read(8)
			if count == 255 {
				count += r.read(8)
			}
			if align != 0 {
				r.pos = (r.pos + 7) &^ 7
			}
			r.pos += uint(count) * 8
		case alacFIL:
			count := r.read(4)
			if count == 15 {
				count += r.read(8) - 1
			}
			r.pos += uint(count) * 8
		case alacEND:
			if samples < 0 {
				return dst, errALACData
			}
			for i := range samples {
				dst = append(dst, float32(d.left[i])*d.bitsPerSample, float32(d.right[i])*d.bitsPerSample)
			}
			return dst, nil
		default:
			return dst, errors.New("alac: unsupported element")
		}
		if ch >= int(d.channels) {
			// Done with the channels: an END element should follow, but
			// some encoders leave it out.
			if r.over() || samples < 0 {
				return dst, errALACData
			}
			for i := range samples {
				dst = append(dst, float32(d.left[i])*d.bitsPerSample, float32(d.right[i])*d.bitsPerSample)
			}
			return dst, nil
		}
	}
}

// element decodes a single channel, or a pair, as channel ch on, and keeps
// what of it plays. It returns its samples a channel.
func (d *alacDecoder) element(r *alacBits, pair bool, ch int) (int, error) {
	r.read(4) // its instance
	if r.read(12) != 0 {
		return 0, errALACData
	}
	header := r.read(4)
	partial := header>>3 != 0
	shifted := uint(header >> 1 & 3)
	escape := header&1 != 0
	if shifted == 3 {
		return 0, errALACData
	}
	n := d.frameLength
	if partial {
		n = r.read(32)
		if n == 0 || n > d.frameLength {
			return 0, errALACData
		}
	}
	chanBits := uint(d.bitDepth) - shifted*8
	if pair {
		chanBits++
	}
	if chanBits > 32 {
		return 0, errALACData
	}
	u, v := d.u[:n], d.v[:n]
	var mixBits uint32
	var mixRes int32
	if !escape {
		mixBits = r.read(8)
		mixRes = int32(int8(r.read(8)))
		modeU, denU, pbU, coefsU := d.predictorHeader(r, &d.coefsU)
		var modeV, denV, pbV uint32
		var coefsV []int16
		if pair {
			modeV, denV, pbV, coefsV = d.predictorHeader(r, &d.coefsV)
		}
		var shiftBits alacBits
		if shifted != 0 {
			shiftBits = *r
			ways := uint(1)
			if pair {
				ways = 2
			}
			r.pos += shifted * 8 * ways * uint(n)
		}
		if err := d.channel(r, u, modeU, denU, pbU, coefsU, chanBits); err != nil {
			return 0, err
		}
		if pair {
			if err := d.channel(r, v, modeV, denV, pbV, coefsV, chanBits); err != nil {
				return 0, err
			}
		}
		if shifted != 0 {
			bits := shifted * 8
			if pair {
				for i := range n {
					d.shift[2*i] = uint16(shiftBits.read(bits))
					d.shift[2*i+1] = uint16(shiftBits.read(bits))
				}
			} else {
				for i := range n {
					d.shift[i] = uint16(shiftBits.read(bits))
				}
			}
		}
	} else {
		// Uncompressed, and never shifted.
		shifted = 0
		chanBits = uint(d.bitDepth)
		ext := 32 - chanBits
		for i := range n {
			u[i] = int32(r.read(chanBits)<<ext) >> ext
			if pair {
				v[i] = int32(r.read(chanBits)<<ext) >> ext
			}
		}
	}
	if r.over() {
		return 0, errALACData
	}
	keep := func(c int, out []int32) {
		switch c {
		case d.leftCh:
			copy(d.left, out)
			if d.rightCh == c {
				copy(d.right, out)
			}
		case d.rightCh:
			copy(d.right, out)
		}
	}
	bits := shifted * 8
	if !pair {
		if shifted != 0 {
			for i := range n {
				u[i] = u[i]<<bits | int32(d.shift[i])
			}
		}
		keep(ch, u)
		return int(n), nil
	}
	for i := range n {
		l, rr := u[i], v[i]
		if mixRes != 0 {
			l = u[i] + v[i] - (mixRes*v[i])>>mixBits
			rr = l - v[i]
		}
		if shifted != 0 {
			l = l<<bits | int32(d.shift[2*i])
			rr = rr<<bits | int32(d.shift[2*i+1])
		}
		u[i], v[i] = l, rr
	}
	keep(ch, u)
	keep(ch+1, v)
	return int(n), nil
}

// predictorHeader reads a channel's predictor: its mode, its shift, its
// rice parameter's factor and its coefficients.
func (d *alacDecoder) predictorHeader(r *alacBits, coefs *[32]int16) (mode, den, pbFactor uint32, c []int16) {
	h := r.read(8)
	mode, den = h>>4, h&15
	h = r.read(8)
	pbFactor = h >> 5
	num := h & 31
	for i := range num {
		coefs[i] = int16(r.read(16))
	}
	return mode, den, pbFactor, coefs[:num]
}

// channel entropy-decodes a channel's residues, and predicts its samples
// from them, into out.
func (d *alacDecoder) channel(r *alacBits, out []int32, mode, den, pbFactor uint32, coefs []int16, chanBits uint) error {
	pc := d.predictor[:len(out)]
	pb := uint32(d.pb) * pbFactor / 4
	if err := d.residues(r, pc, pb, chanBits); err != nil {
		return err
	}
	if mode != 0 {
		unpredict(pc, pc, nil, 31, uint32(chanBits), 0)
	}
	unpredict(pc, out, coefs, len(coefs), uint32(chanBits), den)
	return nil
}

// The adaptive Golomb coder's constants.
const (
	alacQBShift   = 9
	alacQB        = 1 << alacQBShift
	alacMMulShift = 2
	alacMDenShift = alacQBShift - alacMMulShift - 1
	alacMOff      = 1 << (alacMDenShift - 2)
	alacBitOff    = 24
	alacMaxPrefix = 9
)

// residues decodes len(out) residues, adapting the code to their mean as it
// goes, with runs of zeros coded apart where it's small.
func (d *alacDecoder) residues(r *alacBits, out []int32, pb uint32, maxBits uint) error {
	mb := uint32(d.mb)
	kb := uint32(d.kb)
	wb := uint32(1)<<kb - 1
	zmode := uint32(0)
	for c := 0; c < len(out); {
		if r.pos >= r.n {
			return errALACData
		}
		m := mb >> alacQBShift
		k := min(uint32(31-bits.LeadingZeros32(m+3)), kb)
		m = 1<<k - 1
		n := r.golomb32(m, k, maxBits)
		nd := n + zmode
		del := int32((nd + 1) >> 1)
		if nd&1 != 0 {
			del = -del
		}
		out[c] = del
		c++
		mb = pb*(n+zmode) + mb - (pb*mb)>>alacQBShift
		if n > 0xffff {
			mb = 0xffff
		}
		zmode = 0
		if mb<<alacMMulShift < alacQB && c < len(out) {
			zmode = 1
			k := uint32(bits.LeadingZeros32(mb)) - alacBitOff + (mb+alacMOff)>>alacMDenShift
			mz := (uint32(1)<<k - 1) & wb
			run := r.golomb16(mz, k)
			if c+int(run) > len(out) {
				return errALACData
			}
			for range run {
				out[c] = 0
				c++
			}
			if run >= 0xffff {
				zmode = 0
			}
			mb = 0
		}
	}
	return nil
}

// golomb32 reads a residue: a unary prefix, then k bits, or else, after
// alacMaxPrefix ones, the residue in maxBits.
func (r *alacBits) golomb32(m, k uint32, maxBits uint) uint32 {
	word := r.at(r.pos, 32)
	pre := uint32(bits.LeadingZeros32(^word))
	if pre >= alacMaxPrefix {
		v := r.at(r.pos+alacMaxPrefix, maxBits)
		r.pos += alacMaxPrefix + maxBits
		return v
	}
	r.pos += uint(pre) + 1
	if k == 1 {
		return pre
	}
	v := word << (pre + 1) >> (32 - k)
	r.pos += uint(k) - 1
	res := pre * m
	if v >= 2 {
		res += v - 1
		r.pos++
	}
	return res
}

// golomb16 reads the length of a run of zeros, as golomb32 does a residue
// but escaping to 16 bits.
func (r *alacBits) golomb16(m, k uint32) uint32 {
	word := r.at(r.pos, 32)
	pre := uint32(bits.LeadingZeros32(^word))
	if pre >= alacMaxPrefix {
		v := r.at(r.pos+alacMaxPrefix, 16)
		r.pos += alacMaxPrefix + 16
		return v
	}
	v := word << (pre + 1) >> (32 - k)
	r.pos += uint(pre) + 1 + uint(k)
	res := pre*m + v - 1
	if v < 2 {
		res = pre * m
		r.pos--
	}
	return res
}

func signOf(i int32) int32 {
	switch {
	case i > 0:
		return 1
	case i < 0:
		return -1
	}
	return 0
}

// unpredict undoes the adaptive linear prediction of numActive coefficients
// on pc into out, which may be the same. With 31 it's the first-order one.
func unpredict(pc, out []int32, coefs []int16, numActive int, chanBits, denShift uint32) {
	num := len(out)
	if num == 0 {
		return
	}
	shift := 32 - chanBits
	out[0] = pc[0]
	if numActive == 0 {
		copy(out[1:], pc[1:num])
		return
	}
	if numActive == 31 {
		prev := out[0]
		for j := 1; j < num; j++ {
			del := pc[j] + prev
			prev = del << shift >> shift
			out[j] = prev
		}
		return
	}
	for j := 1; j <= numActive && j < num; j++ {
		del := pc[j] + out[j-1]
		out[j] = del << shift >> shift
	}
	var half int32
	if denShift > 0 {
		half = 1 << (denShift - 1)
	}
	lim := numActive + 1
	for j := lim; j < num; j++ {
		top := out[j-lim]
		var sum int32
		for k := range numActive {
			sum += int32(coefs[k]) * (out[j-1-k] - top)
		}
		del := pc[j]
		del0 := del
		sg := signOf(del)
		del += top + (sum+half)>>denShift
		out[j] = del << shift >> shift
		switch {
		case sg > 0:
			for k := numActive - 1; k >= 0; k-- {
				dd := top - out[j-1-k]
				sgn := signOf(dd)
				coefs[k] -= int16(sgn)
				del0 -= int32(numActive-k) * ((sgn * dd) >> denShift)
				if del0 <= 0 {
					break
				}
			}
		case sg < 0:
			for k := numActive - 1; k >= 0; k-- {
				dd := top - out[j-1-k]
				sgn := signOf(dd)
				coefs[k] += int16(sgn)
				del0 -= int32(numActive-k) * ((-sgn * dd) >> denShift)
				if del0 >= 0 {
					break
				}
			}
		}
	}
}

func (d *alacDecoder) SampleRate() int   { return int(d.rate) }
func (d *alacDecoder) Reset()            {}
func (d *alacDecoder) Preroll() int      { return 0 }
func (d *alacDecoder) PacketFrames() int { return 0 }

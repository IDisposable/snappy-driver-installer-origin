package sdwfile

import "encoding/binary"

// bcjX86Decode reverses the LZMA SDK's x86 BCJ (branch conversion)
// filter in place: absolute CALL/JMP targets in x86 machine code that
// were rewritten (to compress better) get restored to their original
// position-relative encoding.
//
// This is a direct translation of the decode path of Bra86.c (Igor
// Pavlov, public domain; external/SevenZ/build/C/Bra86.c in this
// repo), as invoked by Lzma86_Decode in Lzma86Dec.c: a single call
// over the whole buffer, with ip=0 and initial state=0.
//
// Ported label-for-label (using goto) rather than restructured: this
// is a dense, bit-exact algorithm with several early-exit paths, and
// "cleaning it up" risks silently producing wrong bytes instead of an
// error. The one structural change from the C is unavoidable: Go
// forbids a goto from outside a loop into a label inside it, which the
// C relies on once (to enter the loop body without running its
// "mask |= 4" step on the first iteration) - replaced with a boolean
// flag that has the identical effect.
func bcjX86Decode(data []byte) {
	size := len(data)
	if size < 5 {
		return
	}

	p := 0
	lim := size - 4
	mask := uint32(0)    // initial state (Z7_BRANCH_CONV_ST_X86_STATE_INIT_VAL)
	const pc = uint32(0) // ip parameter; Lzma86_Decode always passes 0

	pcGet := func(p int) uint32 { return pc + 4 + uint32(p) }
	needConv := func(b byte) bool { return ((uint32(b) + 1) & 0xfe) == 0 }
	isBCJByte := func(p, n int) bool { return data[p+n-4]&0xfe == 0xe8 }

	firstIter := true
	for {
		if !firstIter {
			mask |= 4
		}
		firstIter = false

		if p >= lim {
			goto fin
		}
		{
			p += 4
			if isBCJByte(p, 0) {
				goto m0
			}
			mask >>= 1
			if isBCJByte(p, 1) {
				goto m1
			}
			mask >>= 1
			if isBCJByte(p, 2) {
				goto m2
			}
			mask = 0
			if isBCJByte(p, 3) {
				goto a3
			}
		}
		goto mainLoop

	m0:
		p--
	m1:
		p--
	m2:
		p--
		if mask == 0 {
			goto a3
		}
		if p > lim {
			goto finP
		}
		if mask > 4 || mask == 3 {
			mask >>= 1
			continue
		}
		mask >>= 1
		if needConv(data[p+int(mask)]) {
			continue
		}
		{
			v := binary.LittleEndian.Uint32(data[p : p+4])
			v += 1 << 24
			if v&0xfe000000 != 0 {
				continue
			}
			c := pcGet(p)
			v -= c
			{
				sh := mask << 3
				if needConv(byte(v >> sh)) {
					v ^= (uint32(0x100) << sh) - 1
					c = pcGet(p)
					v -= c
				}
				mask = 0
			}
			v &= (1 << 25) - 1
			v -= 1 << 24
			binary.LittleEndian.PutUint32(data[p:p+4], v)
			p += 4
			goto mainLoop
		}

	mainLoop:
		if p >= lim {
			goto fin
		}
		for {
			p += 4
			if isBCJByte(p, 0) {
				goto a0
			}
			if isBCJByte(p, 1) {
				goto a1
			}
			if isBCJByte(p, 2) {
				goto a2
			}
			if isBCJByte(p, 3) {
				goto a3
			}
			if p >= lim {
				goto fin
			}
		}

	a0:
		p--
	a1:
		p--
	a2:
		p--
	a3:
		if p > lim {
			goto finP
		}
		{
			v := binary.LittleEndian.Uint32(data[p : p+4])
			v += 1 << 24
			if v&0xfe000000 != 0 {
				continue
			}
			c := pcGet(p)
			v -= c
			v &= (1 << 25) - 1
			v -= 1 << 24
			binary.LittleEndian.PutUint32(data[p:p+4], v)
			p += 4
			goto mainLoop
		}
	}

finP:
	p--
fin:
}

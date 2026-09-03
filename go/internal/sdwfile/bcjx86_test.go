package sdwfile

import (
	"bytes"
	"testing"
)

func TestBCJX86DecodeShortBufferIsNoop(t *testing.T) {
	data := []byte{0xe8, 0x01, 0x02, 0x03}
	orig := bytes.Clone(data)
	bcjX86Decode(data)
	if !bytes.Equal(data, orig) {
		t.Errorf("buffers shorter than 5 bytes should be left untouched, got %x want %x", data, orig)
	}
}

func TestBCJX86DecodeNoOpcodesIsNoop(t *testing.T) {
	data := bytes.Repeat([]byte{0x90}, 64) // NOP sled, no 0xE8/0xE9 bytes
	orig := bytes.Clone(data)
	bcjX86Decode(data)
	if !bytes.Equal(data, orig) {
		t.Errorf("a buffer with no CALL/JMP opcodes should be left untouched")
	}
}

func TestBCJX86DecodeDoesNotPanicOnAllOpcodeBytes(t *testing.T) {
	// Worst case for the scanner: every byte looks like a candidate
	// opcode (0xE8/0xE9), maximizing how often the mask/lookahead logic
	// engages. The real assertion is simply that this doesn't panic
	// (out-of-bounds indexing would be the failure mode of a translation
	// bug) - see sdwfile_test.go's TestDecodeRealIndexFiles for the
	// correctness evidence (real files using this filter decode to
	// sane, correctly-sized payloads).
	for _, n := range []int{5, 6, 7, 8, 9, 16, 17, 100, 101} {
		data := bytes.Repeat([]byte{0xe8}, n)
		bcjX86Decode(data)
	}
}

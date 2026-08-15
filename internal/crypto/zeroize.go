package crypto

import "crypto/subtle"

// Zeroize overwrites a buffer holding sensitive material.
//
// This is a best-effort hygiene measure, not a guarantee. Go's garbage
// collector may have already copied the buffer during a heap move, the value
// may have been spilled to a register or the stack, and the operating system
// may have paged it to disk. Zeroize shortens the window in which key material
// sits in reusable memory; it does not eliminate it.
//
// The write goes through crypto/subtle to keep the compiler from eliminating it
// as a dead store.
func Zeroize(b []byte) {
	if len(b) == 0 {
		return
	}
	subtle.ConstantTimeCopy(1, b, make([]byte, len(b)))
}

// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scanner

import "unicode/utf8"

// Exported for testing only.

var IsSpace = isSpace

func (s *Scanner) Scan() bool {
	return s.Next()
}

func (s *Scanner) MaxTokenSize(n int) {
	if n < utf8.UTFMax || n > 1e9 {
		panic("bad max token size")
	}

	if n < len(s.buf) {
		s.buf = make([]byte, n)
	}
	s.maxTokenSize = n
}

// ErrOrEOF is like Err, but returns EOF. Used to test a corner case.
func (s *Scanner) ErrOrEOF() error {
	return s.err
}

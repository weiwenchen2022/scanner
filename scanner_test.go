// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scanner_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	. "github.com/weiwenchen2022/scanner"
)

const smallMaxTokenSize = 256 // Much smaller for more efficient testing.

// Test white space table matches the Unicode definition.
func TestIsSpace(t *testing.T) {
	t.Parallel()

	for r := rune(0); r <= utf8.MaxRune; r++ {
		if unicode.IsSpace(r) != IsSpace(r) {
			t.Fatalf("white space property disagrees: %#U should be %t", r, unicode.IsSpace(r))
		}
	}
}

var scanTests = []string{
	"",
	"a",
	"¼",
	"☹",
	"\x81",   // UTF-8 error
	"\uFFFD", // correctly encoded RuneError
	"abcdefgh",
	"abc def\n\t\tgh    ",
	"abc¼☹\x81\uFFFD日本語\x82abc",
}

func TestScanByte(t *testing.T) {
	t.Parallel()

	for n, test := range scanTests {
		t.Run(fmt.Sprintf("%d", n), func(t *testing.T) {
			sc := New(strings.NewReader(test))
			sc.Split(SplitBytes)

			i := 0
			for sc.Next() {
				if b := sc.Bytes(); len(b) != 1 || test[i] != b[0] {
					t.Errorf("%d: expected %q got %q", i, test[i], b)
				}
				i++
			}
			if len(test) != i {
				t.Errorf("termination expected at %d; got %d", len(test), i)
			}
			if err := sc.Err(); err != nil {
				t.Error(err)
			}
		})
	}
}

// Test that the rune splitter returns same sequence of runes (not bytes) as for range string.
func TestScanRune(t *testing.T) {
	t.Parallel()

	for n, test := range scanTests {
		t.Run(fmt.Sprintf("%d", n), func(t *testing.T) {
			sc := New(strings.NewReader(test))
			sc.Split(SplitRunes)

			var i int
			var r rune
			runeCount := 0
			// Use a string range loop to validate the sequence of runes.
			for i, r = range string(test) {
				if !sc.Next() {
					break
				}

				runeCount++
				if ch, _ := utf8.DecodeRune(sc.Bytes()); r != ch {
					t.Errorf("%d: expected %q got %q", i, r, ch)
				}
			}

			if sc.Next() {
				t.Errorf("scan ran too long, got %q", sc.Bytes())
			}

			testRuneCount := utf8.RuneCountInString(test)
			if testRuneCount != runeCount {
				t.Errorf("termination expected at %d; got %d", testRuneCount, runeCount)
			}

			if err := sc.Err(); err != nil {
				t.Error(err)
			}
		})
	}
}

var wordScanTests = []string{
	"",
	" ",
	"\n",
	"a",
	" a ",
	"abc def",
	" abc def ",
	" abc\tdef\nghi\rjkl\fmno\vpqr\u0085stu\u00a0\n",
}

// Test that the word splitter returns the same data as strings.Fields.
func TestScanWords(t *testing.T) {
	t.Parallel()

	for n, test := range wordScanTests {
		t.Run(fmt.Sprintf("%d", n), func(t *testing.T) {
			sc := New(strings.NewReader(test))
			sc.Split(SplitWords)

			words := strings.Fields(test)

			wordCount := 0
			for wordCount < len(words) {
				if !sc.Next() {
					break
				}

				if s := sc.Text(); words[wordCount] != s {
					t.Errorf("%d: expected %q got %q", wordCount, words[wordCount], s)
				}
				wordCount++
			}

			if sc.Next() {
				t.Errorf("scan ran too long, got %q", sc.Text())
			}

			if len(words) != wordCount {
				t.Errorf("termination expected at %d; got %d", len(words), wordCount)
			}

			if err := sc.Err(); err != nil {
				t.Error(err)
			}
		})
	}
}

// slowReader is a reader that returns only a few bytes at a time, to test the incremental
// reads in Scanner.Next.
type slowReader struct {
	max int
	r   io.Reader
}

func (sr *slowReader) Read(p []byte) (int, error) {
	if len(p) > sr.max {
		p = p[:sr.max]
	}
	return sr.r.Read(p)
}

// genLine writes to buf a predictable but non-trivial line of text of length
// n, including the terminal newline and an occasional carriage return.
// If addNewline is false, the \r and \n are not emitted.
func genLine(buf *bytes.Buffer, lineNum, n int, addNewline bool) {
	buf.Reset()
	doCR := lineNum%5 == 0
	if doCR {
		n--
	}

	for i := 0; i < n-1; i++ { // Stop early for \n.
		c := 'a' + byte(lineNum+i)
		if c == '\n' || c == '\r' { // Don't confuse us.
			c = 'N'
		}
		buf.WriteByte(c)
	}

	if addNewline {
		if doCR {
			buf.WriteByte('\r')
		}
		buf.WriteByte('\n')
	}
}

// Test the line splitter, including some carriage returns but no long lines.
func TestScanLongLines(t *testing.T) {
	t.Parallel()

	// Build a buffer of lots of line lengths up to but not exceeding smallMaxTokenSize.
	tmp := new(bytes.Buffer)
	buf := new(bytes.Buffer)

	lineNum := 0
	j := 0
	for i := 0; i < 2*smallMaxTokenSize; i++ {
		genLine(tmp, lineNum, j, true)
		if j < smallMaxTokenSize {
			j++
		} else {
			j--
		}

		buf.Write(tmp.Bytes())
		lineNum++
	}

	sc := New(&slowReader{1, buf})
	sc.Split(SplitLines)
	sc.MaxTokenSize(smallMaxTokenSize)

	lineNum = 0
	j = 0
	for sc.Next() {
		genLine(tmp, lineNum, j, false)
		if j < smallMaxTokenSize {
			j++
		} else {
			j--
		}

		line := tmp.String() // We use the string-valued token here, for variety.
		if line != sc.Text() {
			t.Errorf("%d: bad line: %d %d\n%.100q\n%.100q\n", lineNum, len(sc.Text()), len(line), sc.Text(), line)
		}

		lineNum++
	}

	if err := sc.Err(); err != nil {
		t.Error(err)
	}
}

// Test that the line splitter errors out on a long line.
func TestScanLineTooLong(t *testing.T) {
	t.Parallel()

	// const smallMaxTokenSize = 256 // Much smaller for more efficient testing.

	// Build a buffer of lots of line lengths up to but not exceeding smallMaxTokenSize.
	tmp := new(bytes.Buffer)
	buf := new(bytes.Buffer)

	lineNum := 0
	j := 0
	for i := 0; i < 2*smallMaxTokenSize; i++ {
		genLine(tmp, lineNum, j, true)
		j++
		buf.Write(tmp.Bytes())
		lineNum++
	}

	sc := New(&slowReader{3, buf})
	sc.Split(SplitLines)
	sc.MaxTokenSize(smallMaxTokenSize)

	lineNum = 0
	j = 0
	for sc.Next() {
		genLine(tmp, lineNum, j, false)
		if j < smallMaxTokenSize {
			j++
		} else {
			j--
		}

		line := tmp.Bytes()
		if !bytes.Equal(line, sc.Bytes()) {
			t.Errorf("%d: bad line: %d %d\n%.100q\n%.100q\n", lineNum, len(sc.Bytes()), len(line), sc.Bytes(), line)
		}

		lineNum++
	}

	if err := sc.Err(); ErrTooLong != err {
		t.Fatalf("expected ErrTooLong; got %v", err)
	}
}

// Test that the line splitter handles a final line without a newline.
func testNoNewline(t *testing.T, text string, lines []string) {
	r := strings.NewReader(text)
	sc := New(&slowReader{7, r})
	sc.Split(SplitLines)

	var lineNum int
	for lineNum = 0; lineNum < len(lines); lineNum++ {
		if !sc.Next() {
			break
		}

		line := lines[lineNum]
		if line != sc.Text() {
			t.Errorf("%d: bad line: %d %d\n%.100q\n%.100q\n", lineNum, len(sc.Bytes()), len(line), sc.Bytes(), line)
		}
	}
	if sc.Next() {
		t.Errorf("scan ran too long, got %q", sc.Text())
	}
	if len(lines) != lineNum {
		t.Errorf("termination expected at %d; got %d", len(lines), lineNum)
	}

	if err := sc.Err(); err != nil {
		t.Error(err)
	}
}

// Test that the line splitter handles a final line without a newline.
func TestScanLineNoNewline(t *testing.T) {
	t.Parallel()

	const text = "abcdefghijklmn\nopqrstuvwxyz"
	lines := []string{
		"abcdefghijklmn",
		"opqrstuvwxyz",
	}
	testNoNewline(t, text, lines)
}

// Test that the line splitter handles a final line with a carriage return but no newline.
func TestScanLineReturnButNoNewline(t *testing.T) {
	t.Parallel()

	const text = "abcdefghijklmn\nopqrstuvwxyz\r"
	lines := []string{
		"abcdefghijklmn",
		"opqrstuvwxyz",
	}
	testNoNewline(t, text, lines)
}

// Test that the line splitter handles a final empty line.
func TestScanLineEmptyFinalLine(t *testing.T) {
	t.Parallel()

	const text = "abcdefghijklmn\nopqrstuvwxyz\n\n"
	lines := []string{
		"abcdefghijklmn",
		"opqrstuvwxyz",
		"",
	}
	testNoNewline(t, text, lines)
}

// Test that the line splitter handles a final empty line with a carriage return but no newline.
func TestScanLineEmptyFinalLineWithCR(t *testing.T) {
	t.Parallel()

	const text = "abcdefghijklmn\nopqrstuvwxyz\n\r"
	lines := []string{
		"abcdefghijklmn",
		"opqrstuvwxyz",
		"",
	}
	testNoNewline(t, text, lines)
}

var errTest = errors.New("errTest")

// Test the correct error is returned when the split function errors out.
func TestSplitError(t *testing.T) {
	t.Parallel()

	// Create a split function that delivers a little data, then a predictable error.
	numSplits := 0
	const okCount = 7
	errorSplit := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF {
			t.Fatal("didn't get enough data")
		}

		if numSplits >= okCount {
			return 0, nil, errTest
		}

		numSplits++
		return 1, data[:1], nil
	}

	// Read the data.
	const text = "abcdefghijklmnopqrstuvwxyz"
	r := strings.NewReader(text)
	sc := New(&slowReader{1, r})
	sc.Split(errorSplit)

	i := 0
	for sc.Next() {
		if len(sc.Bytes()) != 1 || text[i] != sc.Bytes()[0] {
			t.Errorf("#%d: expected %q got %q", i, text[i], sc.Bytes()[0])
		}
		i++
	}

	// Check correct termination location and error.
	if okCount != i {
		t.Errorf("unexpected termination; expected %d tokens got %d", okCount, i)
	}
	if err := sc.Err(); errTest != err {
		t.Fatalf("expected %v got %v", errTest, err)
	}
}

// Test that an EOF is overridden by a user-generated scan error.
func TestErrAtEOF(t *testing.T) {
	t.Parallel()

	sc := New(strings.NewReader("1 2 33"))
	// This splitter will fail on last entry, after sc.err==EOF.
	split := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		advance, token, err = SplitWords(data, atEOF)
		if len(token) > 1 {
			if io.EOF != sc.ErrOrEOF() {
				t.Fatal("not testing EOF")
			}
			err = errTest
		}
		return
	}
	sc.Split(split)

	for sc.Next() {
	}
	if errTest != sc.Err() {
		t.Error("wrong error:", sc.Err())
	}
}

// Test for issue 5268.
type alwaysError struct{}

func (alwaysError) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestNonEOFWithEmptyRead(t *testing.T) {
	t.Parallel()

	sc := New(alwaysError{})
	for sc.Next() {
		t.Fatal("read should fail")
	}
	if err := sc.Err(); io.ErrUnexpectedEOF != err {
		t.Errorf("unexpected error: %v", err)
	}
}

// Test that Scan finishes if we have endless empty reads.
type endlessZeros struct{}

func (endlessZeros) Read([]byte) (int, error) {
	return 0, nil
}

func TestBadReader(t *testing.T) {
	t.Parallel()

	sc := New(endlessZeros{})
	for sc.Next() {
		t.Fatal("read should fail")
	}
	if err := sc.Err(); io.ErrNoProgress != err {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestScanWordsExcessiveWhiteSpace(t *testing.T) {
	t.Parallel()

	const word = "ipsum"
	s := strings.Repeat(" ", 4*smallMaxTokenSize) + word

	sc := New(strings.NewReader(s))
	sc.MaxTokenSize(smallMaxTokenSize)
	sc.Split(SplitWords)

	if !sc.Next() {
		t.Fatalf("scan failed: %v", sc.Err())
	}
	if token := sc.Text(); word != token {
		t.Fatalf("unexpected token: %q", token)
	}
}

// Test that empty tokens, including at end of line or end of file, are found by the scanner.
// Issue 8672: Could miss final empty token.

func commaSplit(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i := range data {
		if data[i] == ',' {
			return i + 1, data[:i], nil
		}
	}

	return len(data), data, ErrFinalToken
}

func testEmptyTokens(t *testing.T, text string, values []string) {
	sc := New(strings.NewReader(text))
	sc.Split(commaSplit)

	var i int
	for i = 0; sc.Next(); i++ {
		if i >= len(values) {
			t.Fatalf("got %d fields, expected %d", i+1, len(values))
		}
		if values[i] != sc.Text() {
			t.Errorf("%d: expected %q got %q", i, values[i], sc.Bytes())
		}
	}
	if len(values) != i {
		t.Fatalf("got %d fields, expected %d", i, len(values))
	}
	if err := sc.Err(); err != nil {
		t.Error(err)
	}
}

func TestEmptyTokens(t *testing.T) {
	t.Parallel()
	testEmptyTokens(t, "1,2,3,", []string{"1", "2", "3", ""})
}

func TestWithNoEmptyTokens(t *testing.T) {
	t.Parallel()
	testEmptyTokens(t, "1,2,3", []string{"1", "2", "3"})
}

func loopAtEOFSplit(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if len(data) > 0 {
		return 1, data[:1], nil
	}
	return 0, data, nil
}

func TestDontLoopForever(t *testing.T) {
	t.Parallel()

	sc := New(strings.NewReader("abc"))
	sc.Split(loopAtEOFSplit)

	// Expect a panic
	defer func() {
		switch r := recover(); err := r.(type) {
		case nil:
			t.Fatal("should have panicked")
		case string:
			if !strings.Contains(err, "empty tokens") {
				panic(r)
			}
		default:
			panic(r)
		}
	}()

	for count := 0; sc.Next(); count++ {
		if count > 1000 {
			t.Fatal("looping")
		}
	}
	if sc.Err() != nil {
		t.Error("after scan:", sc.Err())
	}
}

func TestBlankLines(t *testing.T) {
	t.Parallel()

	sc := New(strings.NewReader(strings.Repeat("\n", 1000)))
	for count := 0; sc.Next(); count++ {
		if count > 2000 {
			t.Fatal("looping")
		}
	}
	if sc.Err() != nil {
		t.Error("after scan:", sc.Err())
	}
}

type countdown int

func (c *countdown) split(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if *c > 0 {
		*c--
		return 1, data[:1], nil
	}
	return 0, nil, nil
}

// Check that the looping-at-EOF check doesn't trigger for merely empty tokens.
func TestEmptyLinesOK(t *testing.T) {
	t.Parallel()

	c := countdown(10000)
	sc := New(strings.NewReader(strings.Repeat("\n", 10000)))
	sc.Split(c.split)

	for sc.Next() {
	}
	if sc.Err() != nil {
		t.Error("after scan:", sc.Err())
	}
	if c != 0 {
		t.Errorf("stopped with %d left to process", c)
	}
}

// Make sure we can read a huge token if a big enough buffer is provided.
func TestHugeBuffer(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("x", 2*MaxScanTokenSize)
	sc := New(strings.NewReader(text + "\n"))
	sc.Buffer(make([]byte, 100), 3*MaxScanTokenSize)

	for sc.Next() {
		token := sc.Text()
		if text != token {
			t.Errorf("scan got incorrect token of length %d", len(token))
		}
	}
	if sc.Err() != nil {
		t.Error("after scan:", sc.Err())
	}
}

// negativeEOFReader returns an invalid -1 at the end, as though it
// were wrapping the read system call.
type negativeEOFReader int

func (r *negativeEOFReader) Read(p []byte) (int, error) {
	if *r > 0 {
		c := int(*r)
		if len(p) < c {
			c = len(p)
		}
		for i := 0; i < c; i++ {
			p[i] = 'a'
		}
		p[c-1] = '\n'

		*r -= negativeEOFReader(c)
		return c, nil
	}

	return -1, io.EOF
}

// Test that the scanner doesn't panic and returns ErrBadReadCount
// on a reader that returns a negative count of bytes read (issue 38053).
func TestNegativeEOFReader(t *testing.T) {
	t.Parallel()

	r := negativeEOFReader(10)
	sc := New(&r)
	c := 0
	for sc.Next() {
		c++
		if c > 1 {
			t.Error("read too many lines")
			break
		}
	}
	if err := sc.Err(); ErrBadReadCount != err {
		t.Errorf("scanner.Err: got %v, want %v", err, ErrBadReadCount)
	}
}

// largeReader returns an invalid count that is larger than the number
// of bytes requested.
type largeReader struct{}

func (largeReader) Read(p []byte) (int, error) {
	return len(p) + 1, nil
}

// Test that the scanner doesn't panic and returns ErrBadReadCount
// on a reader that returns an impossibly large count of bytes read (issue 38053).
func TestLargeReader(t *testing.T) {
	t.Parallel()

	sc := New(largeReader{})
	for sc.Next() {
	}
	if err := sc.Err(); ErrBadReadCount != err {
		t.Errorf("scanner.Err: got %v, want %v", err, ErrBadReadCount)
	}
}

func TestReset(t *testing.T) {
	t.Parallel()

	sc := New(strings.NewReader("foo foo"))
	sc.Split(SplitWords)
	if !sc.Next() || sc.Text() != "foo" {
		t.Errorf(`token = %q; want "foo"`, sc.Bytes())
	}

	sc.Reset(strings.NewReader("bar bar"))
	sc.Split(SplitLines)
	if !sc.Next() || sc.Text() != "bar bar" {
		t.Errorf(`ReadAll = %q; want "bar bar"`, sc.Bytes())
	}

	*sc = Scanner{} // zero out the Scanner
	sc.Reset(strings.NewReader("bar bar"))
	sc.Split(SplitLines)
	if !sc.Next() || sc.Text() != "bar bar" {
		t.Errorf(`ReadAll = %q; want "bar bar"`, sc.Bytes())
	}
}

func TestTextAllocs(t *testing.T) {
	r := strings.NewReader("       foo       foo        42        42        42        42        42        42        42        42       4.2       4.2       4.2       4.2\n")
	sc := New(r)

	allocs := testing.AllocsPerRun(100, func() {
		r.Seek(0, io.SeekStart)
		sc.Reset(r)

		if !sc.Next() {
			t.Fatal(sc.Err())
		}
		_ = sc.Text()

		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 1 {
		t.Errorf("Unexpected number of allocations, got %f, want 1", allocs)
	}
}

type scannerInterface interface {
	Buffer([]byte, int)

	Scan() bool
	Bytes() []byte
	Text() string

	Err() error
}

type bench struct {
	setup func(b *testing.B)
	perG  func(b *testing.B, newFn func(io.Reader) scannerInterface)
}

func benchScanner(b *testing.B, bench bench) {
	f1 := func(r io.Reader) scannerInterface {
		return bufio.NewScanner(r)
	}
	f2 := func(r io.Reader) scannerInterface {
		return New(r)
	}

	for _, newFn := range []func(io.Reader) scannerInterface{f1, f2} {
		b.Run(fmt.Sprintf("%T", newFn(nil)), func(b *testing.B) {
			if bench.setup != nil {
				bench.setup(b)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				bench.perG(b, newFn)
			}
		})
	}
}

func BenchmarkSplitBytes(b *testing.B) {
	r := strings.NewReader("       foo       foo        42        42        42        42        42        42        42        42       4.2       4.2       4.2       4.2\n")
	buf := make([]byte, MaxScanTokenSize)

	benchScanner(b, bench{
		setup: func(b *testing.B) {
			b.ReportAllocs()
		},

		perG: func(b *testing.B, newFn func(io.Reader) scannerInterface) {
			r.Seek(0, io.SeekStart)
			sc := newFn(r)
			switch sc := sc.(type) {
			case *bufio.Scanner:
				sc.Split(bufio.ScanBytes)
			case *Scanner:
				sc.Split(SplitBytes)
			default:
				b.Fatal("unknown scanner", sc)
			}
			sc.Buffer(buf, MaxScanTokenSize)

			for sc.Scan() {
				_ = sc.Bytes()
			}
			if err := sc.Err(); err != nil {
				b.Fatal(err)
			}
		},
	})
}

func BenchmarkSplitRunes(b *testing.B) {
	r := strings.NewReader("       foo       foo        42        42        42        42        42        42        42        42       4.2       4.2       4.2       4.2\n")
	buf := make([]byte, MaxScanTokenSize)

	benchScanner(b, bench{
		setup: func(b *testing.B) {
			b.ReportAllocs()
		},

		perG: func(b *testing.B, newFn func(io.Reader) scannerInterface) {
			r.Seek(0, io.SeekStart)
			sc := newFn(r)
			switch sc := sc.(type) {
			case *bufio.Scanner:
				sc.Split(bufio.ScanRunes)
			case *Scanner:
				sc.Split(SplitRunes)
			default:
				b.Fatal("unknown scanner", sc)
			}
			sc.Buffer(buf, MaxScanTokenSize)

			for sc.Scan() {
				_ = sc.Text()
			}
			if err := sc.Err(); err != nil {
				b.Fatal(err)
			}
		},
	})
}

func BenchmarkSplitLines(b *testing.B) {
	r := strings.NewReader("       foo       foo        42        42        42        42        42        42        42        42       4.2       4.2       4.2       4.2\n")
	buf := make([]byte, MaxScanTokenSize)

	benchScanner(b, bench{
		setup: func(b *testing.B) {
			b.ReportAllocs()
		},

		perG: func(b *testing.B, newFn func(io.Reader) scannerInterface) {
			r.Seek(0, io.SeekStart)
			sc := newFn(r)
			sc.Buffer(buf, MaxScanTokenSize)

			for sc.Scan() {
				_ = sc.Text()
			}
			if err := sc.Err(); err != nil {
				b.Fatal(err)
			}
		},
	})
}

func BenchmarkSplitWords(b *testing.B) {
	r := strings.NewReader("       foo       foo        42        42        42        42        42        42        42        42       4.2       4.2       4.2       4.2\n")
	buf := make([]byte, MaxScanTokenSize)

	benchScanner(b, bench{
		setup: func(b *testing.B) {
			b.ReportAllocs()
		},

		perG: func(b *testing.B, newFn func(io.Reader) scannerInterface) {
			r.Seek(0, io.SeekStart)
			sc := newFn(r)
			switch sc := sc.(type) {
			case *bufio.Scanner:
				sc.Split(bufio.ScanWords)
			case *Scanner:
				sc.Split(SplitWords)
			default:
				b.Fatal("unknown scanner", sc)
			}
			sc.Buffer(buf, MaxScanTokenSize)

			for sc.Scan() {
				_ = sc.Bytes()
			}
			if err := sc.Err(); err != nil {
				b.Fatal(err)
			}
		},
	})
}

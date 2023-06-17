// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scanner_test

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/weiwenchen2022/scanner"
)

// The simplest use of a Scanner, to read standard input as a set of lines.
func ExampleScanner_lines() {
	sc := scanner.New(os.Stdin)
	for sc.Next() {
		fmt.Println(sc.Text()) // Println will add back the final '\n'
	}

	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}
}

// Return the most recent call to Scan as a []byte.
func ExampleScanner_Bytes() {
	sc := scanner.New(strings.NewReader("gopher"))
	for sc.Next() {
		fmt.Println(len(sc.Bytes()) == 6)
	}

	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "shouldn't see an error scanning a string")
	}
	// Output:
	// true
}

// Use a Scanner to implement a simple word-count utility by scanning the
// input as a sequence of space-delimited tokens.
func ExampleScanner_words() {
	// An artificial input source.
	const input = "Now is the winter of our discontent,\nMade glorious summer by this sun of York.\n"

	sc := scanner.New(strings.NewReader(input))

	// Set the split function for the scanning operation.
	sc.Split(scanner.SplitWords)

	// Count the words.
	count := 0
	for sc.Next() {
		count++
	}

	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading input:", err)
	}
	fmt.Printf("%d\n", count)
	// Output: 15
}

// Use a Scanner with a custom split function (built by wrapping ScanWords) to validate
// 32-bit decimal input.
func ExampleScanner_custom() {
	// An artificial input source.
	const input = "1234 5678 1234567901234567890"

	sc := bufio.NewScanner(strings.NewReader(input))

	// Create a custom split function by wrapping the existing ScanWords function.
	split := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		advance, token, err = scanner.SplitWords(data, atEOF)
		if token != nil && err == nil {
			_, err = strconv.ParseInt(string(token), 10, 32)
		}
		return
	}
	// Set the split function for the scanning operation.
	sc.Split(split)

	// Validate the input
	for sc.Scan() {
		fmt.Printf("%s\n", sc.Text())
	}

	if err := sc.Err(); err != nil {
		fmt.Printf("Invalid input: %s", err)
	}
	// Output:
	// 1234
	// 5678
	// Invalid input: strconv.ParseInt: parsing "1234567901234567890": value out of range
}

// Use a Scanner with a custom split function to parse a comma-separated
// list with an empty final value.
func ExampleScanner_emptyFinalToken() {
	// Comma-separated list; last entry is empty.
	const input = "1,2,3,4,"

	sc := scanner.New(strings.NewReader(input))

	// Define a split function that separates on commas.
	onComma := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		for i := range data {
			if data[i] == ',' {
				return i + 1, data[:i], nil
			}
		}

		if !atEOF {
			return 0, nil, nil
		}

		// There is one final token to be delivered, which may be the empty string.
		// Returning scanner.ErrFinalToken here tells Scan there are no more tokens after this
		// but does not trigger an error to be returned from Scan itself.
		return len(data), data, scanner.ErrFinalToken
	}
	sc.Split(onComma)

	// Scan.
	for sc.Next() {
		fmt.Printf("%q ", sc.Text())
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading input:", err)
	}
	// Output: "1" "2" "3" "4" ""
}

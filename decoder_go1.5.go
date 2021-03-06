// For go 1.5 and below bufio.Scanner.Buffer() did not exist
//+build !go1.6

package sse

import (
	"bufio"
	"bytes"
	"io"
)

// NewDecoder returns a Decoder with a growing buffer.
// Lines are limited to bufio.MaxScanTokenSize - 1.
func NewDecoder(in io.Reader) *Decoder {
	d := &Decoder{scanner: bufio.NewScanner(in), data: new(bytes.Buffer), retry: defaultRetry}
	d.scanner.Split(scanLinesCR) // See scanlines.go
	return d
}

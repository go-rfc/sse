// For go 1.5 and below bufio.Scanner.Buffer() did not exist
//+build go1.6

package sse

import (
	"bufio"
	"bytes"
	"io"
)

// NewDecoder returns a Decoder with a growing buffer.
// Lines are limited to bufio.MaxScanTokenSize - 1.
func NewDecoder(in io.Reader) *Decoder {
	return NewDecoderSize(in, 0)
}

// NewDecoderSize returns a Decoder with a fixed buffer size.
// This constructor is only available on go >= 1.6
func NewDecoderSize(in io.Reader, bufferSize int) *Decoder {
	d := &Decoder{scanner: bufio.NewScanner(in), data: new(bytes.Buffer), retry: defaultRetry}
	if bufferSize > 0 {
		d.scanner.Buffer(make([]byte, bufferSize), bufferSize)
	}
	d.scanner.Split(scanLinesCR) // See scanlines.go
	return d
}

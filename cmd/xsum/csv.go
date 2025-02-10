package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

type csvWriter struct {
	writer       *csv.Writer
	wroteHeaders bool
	algorithms   []string
}

// Close implements xsumWriter.
func (w *csvWriter) Close() error {
	w.writer.Flush()
	return w.writer.Error()
}

// Write implements xsumWriter.
func (w *csvWriter) Write(hostname string, filename string, size int64, sums map[string][]byte, sErr error) error {
	if !w.wroteHeaders {
		headers := []string{"hostname", "filename", "size", "error"}
		for _, algorithm := range w.algorithms {
			headers = append(headers, algorithm+"sum")
		}
		w.writer.Write(headers)
		w.wroteHeaders = true
	}

	data := []string{hostname, filename, strconv.FormatInt(size, 10)}
	if sErr != nil {
		data = append(data, sErr.Error())
	} else {
		data = append(data, "")
	}

	for _, algorithm := range w.algorithms {
		sum := sums[algorithm]
		if sum == nil {
			data = append(data, "")
		} else {
			data = append(data, fmt.Sprintf("%x", sum))
		}
	}

	return w.writer.Write(data)
}

func newCsvWriter(w io.Writer, algorithms []string) *csvWriter {
	return &csvWriter{csv.NewWriter(w), false, algorithms}
}

var _ xsumWriter = new(csvWriter)

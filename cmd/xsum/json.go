package main

import (
	"encoding/json"
	"io"
)

type jsonWriter struct {
	writer io.Writer
}

// Close implements xsumWriter.
func (*jsonWriter) Close() error {
	return nil
}

// Write implements xsumWriter.
func (w *jsonWriter) Write(hostname string, filename string, size int64, sums map[string][]byte, err error) error {
	data := map[string]any{
		"hostname": hostname,
		"filename": filename,
		"size":     size,
	}
	if err != nil {
		data["error"] = err.Error()
	} else {
		for algorithm, sum := range sums {
			data[algorithm+"sum"] = sum
		}
	}
	return json.NewEncoder(w.writer).Encode(data)
}

var _ xsumWriter = new(jsonWriter)

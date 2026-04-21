package main

import (
	"encoding/json"
	"fmt"
	"io"
)

type jsonWriter struct {
	enc *json.Encoder
}

func newJSONWriter(w io.Writer) *jsonWriter {
	return &jsonWriter{enc: json.NewEncoder(w)}
}

// Close implements xsumWriter.
func (*jsonWriter) Close() error { return nil }

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
			data[algorithm+"sum"] = fmt.Sprintf("%x", sum)
		}
	}
	return w.enc.Encode(data)
}

var _ xsumWriter = new(jsonWriter)

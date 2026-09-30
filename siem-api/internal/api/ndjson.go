package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// maxIngestBodyBytes caps a single Vector http-sink request body. Matches
// Vector's own default batch.max_bytes (10MB), so a full default-sized
// batch is still accepted.
const maxIngestBodyBytes = 10 << 20

// decodeJSONStream decodes every JSON value in r's body into a []T.
//
// Vector's http sink batches events (by default flushing about once a
// second), and `framing.method = "newline_delimited"` joins a batch's
// events with newlines into ONE request body - it does not give each
// event its own request. A single json.Decode would read only the first
// event and silently drop the rest of the batch, so the ingest handlers
// must drain the whole stream. Accepts a lone object too (curl, tests).
//
// All values are decoded before any is returned, so a malformed value
// anywhere rejects the whole body rather than half-applying it.
func decodeJSONStream[T any](w http.ResponseWriter, r *http.Request) ([]T, error) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIngestBodyBytes))
	var out []T
	for {
		var v T
		if err := dec.Decode(&v); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, errors.New("empty body")
	}
	return out, nil
}

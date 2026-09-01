package httpx

import (
	"bytes"
	"net/http"
)

type responseBuffer struct {
	header http.Header
	body   bytes.Buffer
	status int
}

// Header handles the header operation.
func (buffer *responseBuffer) Header() http.Header {
	if buffer.header == nil {
		buffer.header = make(http.Header)
	}
	return buffer.header
}

// WriteHeader writes header.
func (buffer *responseBuffer) WriteHeader(status int) {
	if buffer.status == 0 {
		buffer.status = status
	}
}

// Write writes the supplied data.
func (buffer *responseBuffer) Write(body []byte) (int, error) {
	if buffer.status == 0 {
		buffer.status = http.StatusOK
	}
	return buffer.body.Write(body)
}

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

func (buffer *responseBuffer) Header() http.Header {
	if buffer.header == nil {
		buffer.header = make(http.Header)
	}
	return buffer.header
}

func (buffer *responseBuffer) WriteHeader(status int)         { buffer.status = status }
func (buffer *responseBuffer) Write(body []byte) (int, error) { return buffer.body.Write(body) }

package httpx

import (
	"bytes"
	"net/http"
)

type responseBuffer struct {
	header    http.Header
	body      bytes.Buffer
	status    int
	writer    http.ResponseWriter
	committed bool
}

func (buffer *responseBuffer) Header() http.Header {
	if buffer.header == nil {
		buffer.header = make(http.Header)
	}
	return buffer.header
}

func (buffer *responseBuffer) WriteHeader(status int) {
	if buffer.committed {
		return
	}
	if buffer.status == 0 {
		buffer.status = status
	}
}

func (buffer *responseBuffer) Write(body []byte) (int, error) {
	if buffer.committed {
		return buffer.writer.Write(body)
	}
	if buffer.status == 0 {
		buffer.status = http.StatusOK
	}
	return buffer.body.Write(body)
}

// FlushError switches recovery from atomic responses to streaming. Once bytes
// are sent, a panic must close the stream rather than append a JSON problem.
func (buffer *responseBuffer) FlushError() error {
	if err := buffer.commit(); err != nil {
		return err
	}
	return http.NewResponseController(buffer.writer).Flush()
}

func (buffer *responseBuffer) Unwrap() http.ResponseWriter { return buffer.writer }

func (buffer *responseBuffer) commit() error {
	if buffer.committed {
		return nil
	}
	copyHeader(buffer.writer.Header(), buffer.Header())
	status := buffer.status
	if status == 0 {
		status = http.StatusOK
	}
	buffer.writer.WriteHeader(status)
	buffer.committed = true
	_, err := buffer.writer.Write(buffer.body.Bytes())
	buffer.body.Reset()
	return err
}

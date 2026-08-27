package httpx

import "net/http"

// Recover buffers a response until the handler returns so a panic cannot leave
// a partial success response in front of the RFC 9457 failure.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var buffer responseBuffer
		defer func() {
			if recovered := recover(); recovered != nil {
				_ = WriteProblem(writer, request, NewError(ProblemInternalFailure, nil))
				return
			}
			copyHeader(writer.Header(), buffer.Header())
			status := buffer.status
			if status == 0 {
				status = http.StatusOK
			}
			writer.WriteHeader(status)
			_, _ = writer.Write(buffer.body.Bytes())
		}()
		next.ServeHTTP(&buffer, request)
	})
}

func copyHeader(destination, source http.Header) {
	for name, values := range source {
		destination[name] = append([]string(nil), values...)
	}
}

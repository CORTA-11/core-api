package httpx

import "net/http"

// Recover buffers a response until the handler returns so a panic cannot leave
// a partial success response in front of the RFC 9457 failure. An explicit
// flush opts into streaming, after which failures terminate the response.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		buffer := responseBuffer{writer: writer}
		defer func() {
			if recovered := recover(); recovered != nil {
				if buffer.committed {
					panic(http.ErrAbortHandler)
				}
				_ = WriteProblem(writer, request, NewError(ProblemInternalFailure, nil))
				return
			}
			_ = buffer.commit()
		}()
		next.ServeHTTP(&buffer, request)
	})
}

func copyHeader(destination, source http.Header) {
	for name, values := range source {
		destination[name] = append([]string(nil), values...)
	}
}

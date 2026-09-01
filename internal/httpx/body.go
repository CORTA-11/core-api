package httpx

import (
	"errors"
	"io"
	"net/http"

	"github.com/CORTA-11/core-api/internal/apicontract"
)

const maximumDrainBytes = 4 << 10

// BodyLimitBytes bodys limit bytes.
func BodyLimitBytes(class apicontract.BodyLimitClass) int64 {
	switch class {
	case apicontract.BodyAuthJSON:
		return 4 << 10
	case apicontract.BodyJSON:
		return 64 << 10
	case apicontract.BodyFile:
		return 10 << 20 // 10 MiB
	case apicontract.BodyNone:
		return 0
	default:
		return -1
	}
}

// LimitBody limits body.
func LimitBody(class apicontract.BodyLimitClass, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		limit := BodyLimitBytes(class)
		if limit < 0 {
			_ = WriteProblem(writer, request, NewError(ProblemInternalFailure, nil))
			return
		}
		if limit == 0 {
			if request.ContentLength > 0 || len(request.TransferEncoding) > 0 {
				drainAndClose(request.Body)
				_ = WriteProblem(writer, request, DecodeProblem(ErrBodyNotAllowed))
				return
			}
			next.ServeHTTP(writer, request)
			return
		}
		request.Body = http.MaxBytesReader(writer, request.Body, limit)
		defer drainAndClose(request.Body)
		next.ServeHTTP(writer, request)
	})
}

// drainAndClose drains and close.
func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maximumDrainBytes))
	_ = body.Close()
}

// isMaxBytesError checks whether max bytes error.
func isMaxBytesError(err error) bool {
	var maximum *http.MaxBytesError
	return errors.As(err, &maximum)
}

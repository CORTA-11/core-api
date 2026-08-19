package httpx

import (
	"errors"
	"net/http"
)

const internalDetail = "An unexpected error occurred."

type AppError struct {
	Status int
	Type   string
	Title  string
	Detail string
	Err    error
}

func (err *AppError) Error() string {
	if err.Err != nil {
		return err.Err.Error()
	}
	return err.Title
}

func (err *AppError) Unwrap() error { return err.Err }

type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func WriteProblem(writer http.ResponseWriter, request *http.Request, err error) error {
	problem := Problem{
		Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError),
		Status: http.StatusInternalServerError, Detail: internalDetail, RequestID: RequestIDFromContext(request.Context()),
	}
	var appError *AppError
	if errors.As(err, &appError) && appError.Status >= 400 && appError.Status <= 599 {
		problem.Status = appError.Status
		problem.Title = appError.Title
		problem.Detail = appError.Detail
		if appError.Type != "" {
			problem.Type = appError.Type
		}
	}
	var buffer responseBuffer
	if err := WriteJSON(&buffer, problem.Status, problem); err != nil {
		return err
	}
	writer.Header().Set("Content-Type", "application/problem+json")
	writer.WriteHeader(problem.Status)
	_, writeErr := writer.Write(buffer.body.Bytes())
	return writeErr
}

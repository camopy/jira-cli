package cli

import (
	"encoding/json"
	"io"
)

type ErrorType string

const (
	ErrorTypeAuth       ErrorType = "auth"
	ErrorTypeNotFound   ErrorType = "not_found"
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeRateLimit  ErrorType = "rate_limit"
	ErrorTypeServer     ErrorType = "server"
)

func NewError(kind ErrorType, msg string) Error {
	return Error{Type: string(kind), Message: msg}
}

func ExitCode(err Error) int {
	switch ErrorType(err.Type) {
	case ErrorTypeAuth:
		return 1
	case ErrorTypeNotFound:
		return 2
	case ErrorTypeValidation:
		return 3
	case ErrorTypeRateLimit:
		return 4
	case ErrorTypeServer:
		return 5
	default:
		return 5
	}
}

func WriteErrors(w io.Writer, errors []Error) error {
	b, err := json.Marshal(errors)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

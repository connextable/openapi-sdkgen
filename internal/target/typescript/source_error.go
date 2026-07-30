package typescript

import (
	"errors"
	"fmt"
)

type sourcePointerError struct {
	pointer string
	cause   error
}

func (value *sourcePointerError) Error() string {
	return value.pointer + ": " + value.cause.Error()
}

func (value *sourcePointerError) Unwrap() error {
	return value.cause
}

func withSourcePointer(pointer, format string, arguments ...any) error {
	return &sourcePointerError{pointer: pointer, cause: fmt.Errorf(format, arguments...)}
}

func sourcePointerErrorDetails(err error) (string, string) {
	var located *sourcePointerError
	if errors.As(err, &located) {
		return located.pointer, located.cause.Error()
	}
	return splitPointerError(err.Error())
}

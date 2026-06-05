package errors

import (
	"errors"
	"fmt"
)

type BizError struct {
	Code       Code
	MessageKey string
	Message    string
	Cause      error
}

func New(code Code, messageKey, message string) *BizError {
	return &BizError{Code: code, MessageKey: messageKey, Message: message}
}

func Wrap(cause error, code Code, messageKey, message string) *BizError {
	return &BizError{Code: code, MessageKey: messageKey, Message: message, Cause: cause}
}

func (e *BizError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *BizError) Unwrap() error { return e.Cause }

// AsBiz pulls a BizError out of a wrapped error chain.
func AsBiz(err error) (*BizError, bool) {
	var be *BizError
	if errors.As(err, &be) {
		return be, true
	}
	return nil, false
}

// HTTPStatus maps a business Code to an HTTP status code.
func HTTPStatus(c Code) int {
	switch {
	case c == CodeOK:
		return 200
	case c >= 10000 && c < 20000:
		return 500
	case c == CodeRateLimited:
		return 429
	case c >= 40400 && c < 40500:
		return 404
	case c >= 40300 && c < 40400:
		return 403
	case c >= 40100 && c < 40200:
		return 401
	case c >= 40900 && c < 41000:
		return 409
	case c >= 40000 && c < 41000:
		return 400
	case c >= 50000 && c < 60000:
		return 500
	default:
		return 500
	}
}

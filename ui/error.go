package ui

import "fmt"

type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
	SeverityFatal
)

type AppError struct {
	Code        string
	UserMessage string
	InternalLog string
	Level       Severity
	Err         error // Underlying error
}

func (e *AppError) Error() string {
	if e.InternalLog != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.UserMessage, e.InternalLog)
	}
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.UserMessage, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.UserMessage)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func NewAppError(code, userMessage, internalLog string, level Severity, err error) *AppError {
	return &AppError{
		Code:        code,
		UserMessage: userMessage,
		InternalLog: internalLog,
		Level:       level,
		Err:         err,
	}
}

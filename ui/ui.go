package ui

type Printer interface {
	Success(msg string)
	Error(err error)
	Warning(msg string)
	Info(msg string)
	Print(msg string)
	Printf(format string, args ...interface{})
}

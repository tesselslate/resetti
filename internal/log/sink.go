package log

// Sink is the base on which `Console` and `File` are implemented.
// Contains all the function signatures for each log level.
type Sink interface {
	Error(string) error
	Warn(string) error
	Info(string) error
	Debug(string) error
	Verbose(string) error
}

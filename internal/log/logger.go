package log

import (
	"fmt"
	"os"
)

type LogLevel int

// The level of visibility of the log output.
// ERROR is the lowest level, VERBOSE is the higest and it increases in the order that it is written.
const (
	ERROR LogLevel = iota
	WARN
	INFO
	DEBUG
	VERBOSE
)

// Logger is exposed to the user and all logging is done through it.
// It handles its internal errors, so the user doesn't have to catch any.
// It maintains LogLevel data and Sink data.
// Has functions like Error(), Warn() etc. to print the corresponding log message.
// Logs are printed out to console as well as the log file.
type Logger struct {
	level   LogLevel
	console Sink
	file    Sink
}

// NewLogger creates a fresh instance of Logger and returns it.
// It opens the log file in `filePath` with write-only, truncate and create flags and with mode 0644 (before umask).
func NewLogger(level LogLevel, filePath string, formatter Formatter) Logger {
	logFile, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Couldn't create log file: %s\n", filePath)
		os.Exit(1)
	}

	fileSink := &File{logFile: logFile, formatter: formatter}
	consoleSink := &Console{formatter: formatter}
	return Logger{level: level, file: fileSink, console: consoleSink}
}

// SetLevel sets the log visibility level of the Logger instance.
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// Error prints out the error message passed to the Sinks.
func (l *Logger) Error(message string) {
	err := l.console.Error(message)
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
	err = l.file.Error(message)
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
}

// Warn prints out the warning message passed to the Sinks.
// It also checks if the log level allows for the log to be printed.
func (l *Logger) Warn(message string) {
	if l.level < WARN {
		return
	}
	err := l.console.Warn(message)
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
	err = l.file.Warn(message)
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
}

// Info prints out the information passed to the Sinks.
// It also checks if the log level allows for the log to be printed.
func (l *Logger) Info(message string) {
	if l.level < INFO {
		return
	}
	err := l.console.Info(message)
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
	err = l.file.Info(message)
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
}

// Debug prints out the debug message passed to the Sinks.
// It also checks if the log level allows for the log to be printed.
func (l *Logger) Debug(message string) {
	if l.level < DEBUG {
		return
	}
	err := l.console.Debug(message)
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
	err = l.file.Info(message)
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
}

// Verbose prints out the message passed to the Sinks.
// It also checks if the log level allows for the log to be printed.
func (l *Logger) Verbose(message string) {
	if l.level < VERBOSE {
		return
	}
	err := l.console.Verbose(message)
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
	err = l.file.Verbose(message)
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
}

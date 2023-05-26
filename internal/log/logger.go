package log

import (
	"fmt"
	"io"
	"os"
)

type LogLevel int

// The level of visibility of the log output.
// ERROR is the lowest level, VERBOSE is the highest and it increases in the order that it is written.
const (
	ERROR LogLevel = iota
	WARN
	INFO
	DEBUG
	VERBOSE
)

// Logger is exposed to the user and all logging is done through it.
// It handles its internal errors, so the user doesn't have to catch any.
// It maintains LogLevel data, a Formatter instance and a Writer instance.
// Has functions like Error(), Warn() etc. to print the corresponding log message.
// Logs are printed out to console as well as the log file.
type Logger struct {
	level     LogLevel
	formatter Formatter
	logFile   *os.File
	logWriter io.Writer
}

// DefaultLogger creates a pre-defined instance of Logger with a default formatter.
func DefaultLogger(level LogLevel, filePath string) Logger {
	return NewLogger(level, filePath, DefaultFormatter())
}

// NewLogger creates a fresh instance of Logger with a user-defined Formatter.
// It opens the log file in `filePath` with write-only, truncate and create flags and with mode 0644 (before umask).
func NewLogger(level LogLevel, filePath string, formatter Formatter) Logger {
	logFile, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Couldn't create log file: %s\n", filePath)
		os.Exit(1)
	}
	logWriter := io.MultiWriter(logFile, os.Stdout)
	return Logger{level: level, formatter: formatter, logFile: logFile, logWriter: logWriter}
}

// SetLevel sets the log visibility level of the Logger instance.
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// Write formats the message and flushes it to the Sinks using io.Writer
func (l *Logger) Write(level string, message string) error {
	formattedStr, err := l.formatter.Format(level, message)
	if err != nil {
		return fmt.Errorf("Format failed: %s", err)
	}
	byteStr := []byte(formattedStr)
	_, err = l.logWriter.Write(byteStr)
	if err != nil {
		return fmt.Errorf("Failed to write logs: %s", err)
	}
	return nil
}

// Error prints out the error message passed to the Sinks.
func (l *Logger) Error(message string) {
	err := l.Write("ERROR", message)
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
	err := l.Write("WARN", message)
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
	err := l.Write("INFO", message)
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
	err := l.Write("DEBUG", message)
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
	err := l.Write("VERBOSE", message)
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
}

// Close is used to close the file pointer.
func (l *Logger) Close() {
	err := l.logFile.Close()
	if err != nil {
		fmt.Printf("Failed to close log file: %s\n", err)
		os.Exit(1)
	}
}

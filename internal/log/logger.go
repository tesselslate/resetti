package log

import (
	"fmt"
	"io"
	"os"
	"strings"
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
	conf      LogConf
	level     LogLevel
	formatter Formatter
	logFile   *os.File
	logWriter io.Writer
}

// DefaultLogger creates a pre-defined instance of Logger with a default formatter.
func DefaultLogger(name string, level LogLevel, filePath string) Logger {
	return NewLogger(name, level, filePath, DefaultFormatter())
}

// NewLogger creates a fresh instance of Logger with a user-defined Formatter.
// It opens the log file in `filePath` with write-only, truncate and create flags and with mode 0644 (before umask).
func NewLogger(name string, level LogLevel, filePath string, formatter Formatter) Logger {
	logFile, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Couldn't create log file: %s\n", err)
		os.Exit(1)
	}
	logWriter := io.MultiWriter(logFile, os.Stdout)
	conf := LogConf{Name: name, LogLevel: level, FilePath: filePath, FormatStr: formatter.formatStr}
	err = conf.Write()
	if err != nil {
		fmt.Printf("Couldn't create conf file: %s\n", err)
		os.Exit(1)
	}
	return Logger{conf: conf, level: level, formatter: formatter, logFile: logFile, logWriter: logWriter}
}

// FromName loads an existing Logger instance from disk.
// It parses the conf file in `/tmp/<name>.json` and builds a new Logger instance.
func FromName(name string) Logger {
	conf, err := ConfRead(name)
	if err != nil {
		fmt.Printf("Conf error: %s", err)
		os.Exit(1)
	}
	logFile, err := os.OpenFile(conf.FilePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Couldn't open log file: %s\n", err)
		os.Exit(1)
	}
	logWriter := io.MultiWriter(logFile, os.Stdout)
	return Logger{conf: conf, level: conf.LogLevel, formatter: NewFormatter(conf.FormatStr), logFile: logFile, logWriter: logWriter}
}

// SetLevel sets the log visibility level of the Logger instance.
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
	err := l.conf.UpdateLevel(level)
	if err != nil {
		fmt.Printf("Log update error: %s\n", err)
		os.Exit(1)
	}
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
func (l *Logger) Error(message string, args ...any) {
	err := l.Write("ERROR", fmt.Sprintf(message, args...))
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
}

// Warn prints out the warning message passed to the Sinks.
// It also checks if the log level allows for the log to be printed.
func (l *Logger) Warn(message string, args ...any) {
	if l.level < WARN {
		return
	}
	err := l.Write("WARN", fmt.Sprintf(message, args...))
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
}

// Info prints out the information passed to the Sinks.
// It also checks if the log level allows for the log to be printed.
func (l *Logger) Info(message string, args ...any) {
	if l.level < INFO {
		return
	}
	err := l.Write("INFO", fmt.Sprintf(message, args...))
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
}

// Debug prints out the debug message passed to the Sinks.
// It also checks if the log level allows for the log to be printed.
func (l *Logger) Debug(message string, args ...any) {
	if l.level < DEBUG {
		return
	}
	err := l.Write("DEBUG", fmt.Sprintf(message, args...))
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
}

// Verbose prints out the message passed to the Sinks.
// It also checks if the log level allows for the log to be printed.
func (l *Logger) Verbose(message string, args ...any) {
	if l.level < VERBOSE {
		return
	}
	err := l.Write("VERBOSE", fmt.Sprintf(message, args...))
	if err != nil {
		fmt.Printf("Failed Log Write: %s\n", err)
		os.Exit(1)
	}
}

// GetName is a helper function that reads `/tmp` and gets the name of the current logger and returns it.
func GetName() (string, error) {
	listings, err := os.ReadDir("/tmp/")
	if err != nil {
		return "", fmt.Errorf("Failed to read directory '/tmp/': %s\n", err)
	}
	name := ""
	for _, listing := range listings {
		if strings.Contains(listing.Name(), ".json") {
			name = strings.Split(listing.Name(), ".")[0]
			break
		}
	}
	if name == "" {
		return "", fmt.Errorf("Unable to find any log configs in '/tmp/`: %s\n", err)
	}
	return name, nil
}

// Error is a wrapper function that re-creates the logger instance from config and writes to it.
func Error(message string, args ...any) {
	name, err := GetName()
	if err != nil {
		fmt.Printf("Failed to get name: %s\n", err)
		os.Exit(1)
	}
	logger := FromName(name)
	logger.Error(message, args...)
}

// Warn is a wrapper function that re-creates the logger instance from config and writes to it.
func Warn(message string, args ...any) {
	name, err := GetName()
	if err != nil {
		fmt.Printf("Failed to get name: %s\n", err)
		os.Exit(1)
	}
	logger := FromName(name)
	logger.Warn(message, args...)
}

// Info is a wrapper function that re-creates the logger instance from config and writes to it.
func Info(message string, args ...any) {
	name, err := GetName()
	if err != nil {
		fmt.Printf("Failed to get name: %s\n", err)
		os.Exit(1)
	}
	logger := FromName(name)
	logger.Info(message, args...)
}

// Debug is a wrapper function that re-creates the logger instance from config and writes to it.
func Debug(message string, args ...any) {
	name, err := GetName()
	if err != nil {
		fmt.Printf("Failed to get name: %s\n", err)
		os.Exit(1)
	}
	logger := FromName(name)
	logger.Debug(message, args...)
}

// Verbose is a wrapper function that re-creates the logger instance from config and writes to it.
func Verbose(message string, args ...any) {
	name, err := GetName()
	if err != nil {
		fmt.Printf("Failed to get name: %s\n", err)
		os.Exit(1)
	}
	logger := FromName(name)
	logger.Verbose(message, args...)
}

// Close is used to close the file pointer and deletes the conf file.
func (l *Logger) Close() {
	err := l.logFile.Close()
	if err != nil {
		fmt.Printf("Failed to close log file: %s\n", err)
		os.Exit(1)
	}
	confFile := fmt.Sprintf("/tmp/%s.json", l.conf.Name)
	_, err = os.Stat(confFile)
	if err != nil {
		return
	}
	err = os.Remove(confFile)
	if err != nil {
		fmt.Printf("Failed to remove conf file: %s\n", err)
		os.Exit(1)
	}
}

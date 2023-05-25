package log

import (
	"fmt"
	"os"
)

// File is a Sink for the Logger.
// It is used to control log file writes using its formatter.
// Stores a pointer to the opened logFile and the instance of the formatter created.
type File struct {
	logFile   *os.File
	formatter Formatter
}

// FileWrite writes the formatted output to the log file.
func (f *File) FileWrite(level string, message string) error {
	formattedMsg, err := f.formatter.Format(level, message)
	if err != nil {
		return fmt.Errorf("Format failed: %s", err)
	}
	_, err = f.logFile.WriteString(formattedMsg)
	if err != nil {
		return fmt.Errorf("Unable to write to log file: %s", err)
	}
	return nil
}

// Error writes the message as an ERROR entry in the log file.
func (f *File) Error(message string) error {
	return f.FileWrite("ERROR", message)
}

// Warn writes the message as a WARN entry in the log file.
func (f *File) Warn(message string) error {
	return f.FileWrite("WARN", message)
}

// Info writes the message as a INFO entry in the log file.
func (f *File) Info(message string) error {
	return f.FileWrite("INFO", message)
}

// Debug writes the message as a DEBUG entry in the log file.
func (f *File) Debug(message string) error {
	return f.FileWrite("DEBUG", message)
}

// Verbose writes the message as a VERBOSE entry in the log file.
func (f *File) Verbose(message string) error {
	return f.FileWrite("VERBOSE", message)
}

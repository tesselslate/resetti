package log

import (
	"fmt"
	"strings"
	"time"
)

// Formatter is used by the Sinks to format the log before print.
// It is initialized with a formatStr that can use certain internal variables:
// `ascTime` - The time of the log print in human readable form.
// `level` - The visibility level of the log.
// `message` - The log message itself. This is a compulsory format variable.
// All format variables are enclosed in '{' and '}'.
// Eg: "{ascTime}: [{level}] - {message}"
type Formatter struct {
	formatStr string
}

// DefaultFormatter creates a simple Formatter instance with a pre-defined `formatStr`.
func DefaultFormatter() Formatter {
	return NewFormatter("{ascTime}: [{level}] - {message}")
}

// NewFormatter creates a Formatter instance with a user-defined `formatStr`.
func NewFormatter(formatStr string) Formatter {
	return Formatter{formatStr: formatStr}
}

// GetValues makes an array of strings containing all the format fields with their respective values.
// Used internally to get the recognized format variables in `formatStr`.
func (f *Formatter) GetValues(ascTime string, level string, message string) (map[string]string, error) {
	formatArgs := map[string]string{}
	if strings.Contains(f.formatStr, "{ascTime}") {
		formatArgs["{ascTime}"] = ascTime
	}
	if strings.Contains(f.formatStr, "{level}") {
		formatArgs["{level}"] = level
	}
	if !strings.Contains(f.formatStr, "{message}") {
		return map[string]string{}, fmt.Errorf("Missing `message` parameter in format string.")
	}
	formatArgs["{message}"] = message
	return formatArgs, nil
}

// Format is used to get a fully formatted string from `formatStr`.
// It replaces all variables with their values in `formatStr`.
func (f *Formatter) Format(level string, message string) (string, error) {
	ascTime := time.Now().Format(time.RFC3339)
	values, err := f.GetValues(ascTime, level, message)
	if err != nil {
		return "", fmt.Errorf("Failed Format: %s", err)
	}
	formattedStr := f.formatStr
	for field, value := range values {
		formattedStr = strings.ReplaceAll(formattedStr, field, value)
	}
	return formattedStr + "\n", nil
}

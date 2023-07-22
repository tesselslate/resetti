package log

import (
	"fmt"
	"strings"
	"time"
)

// Color constants for logging.
const (
	Reset  string = "\033[0m"
	White  string = "\033[97m"
	Red    string = "\033[31m"
	Cyan   string = "\033[36m"
	Blue   string = "\033[34m"
	Yellow string = "\033[33m"
)

// GetValues makes an array of strings containing all the format fields with their respective values.
// Used internally to get the recognized format variables in `formatStr`.
func GetValues(ascTime string, level string, message string, formatStr string) (map[string]string, error) {
	formatArgs := map[string]string{}
	if strings.Contains(formatStr, "{ascTime}") {
		formatArgs["{ascTime}"] = ascTime
	}
	if strings.Contains(formatStr, "{level}") {
		formatArgs["{level}"] = level
	}
	if !strings.Contains(formatStr, "{message}") {
		return map[string]string{}, fmt.Errorf("Missing `message` parameter in format string.")
	}
	formatArgs["{message}"] = message
	return formatArgs, nil
}

// Format is used to get a fully formatted string from `formatStr`.
// It replaces all variables with their values in `formatStr`.
func Format(level string, message string, formatStr string) (string, error) {
	ascTime := time.Now().Format(time.RFC3339)
	values, err := GetValues(ascTime, level, message, formatStr)
	if err != nil {
		return "", fmt.Errorf("Failed Format: %s", err)
	}
	for field, value := range values {
		formatStr = strings.ReplaceAll(formatStr, field, value)
	}
	switch level {
	case "ERROR":
		formatStr = Red + formatStr + Reset
	case "WARN":
		formatStr = Yellow + formatStr + Reset
	case "INFO":
		formatStr = White + formatStr + Reset
	case "DEBUG":
		formatStr = Cyan + formatStr + Reset
	case "VERBOSE":
		formatStr = Blue + formatStr + Reset
	}
	return formatStr + "\n", nil
}

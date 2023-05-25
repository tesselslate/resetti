package log

import (
	"fmt"
)

// Console is a Sink for the Logger.
// It is used to control writes to the console using its formatter.
// Stores the instance of the formatter created.
type Console struct {
	formatter Formatter
}

// ConsoleWrite writes the formatted output to the console.
func (c *Console) ConsoleWrite(level string, message string) error {
	formattedMsg, err := c.formatter.Format(level, message)
	if err != nil {
		return fmt.Errorf("Format failed: %s", err)
	}
	_, err = fmt.Printf("%s", formattedMsg)
	if err != nil {
		return fmt.Errorf("Unable to write to console: %s", err)
	}
	return nil
}

// Error writes the message as an ERROR entry in the console.
func (c *Console) Error(message string) error {
	return c.ConsoleWrite("ERROR", message)
}

// Warn writes the message as a WARN entry in the console.
func (c *Console) Warn(message string) error {
	return c.ConsoleWrite("WARN", message)
}

// Info writes the message as a INFO entry in the console.
func (c *Console) Info(message string) error {
	return c.ConsoleWrite("INFO", message)
}

// Debug writes the message as a DEBUG entry in the console.
func (c *Console) Debug(message string) error {
	return c.ConsoleWrite("DEBUG", message)
}

// Verbose writes the message as a VERBOSE entry in the console.
func (c *Console) Verbose(message string) error {
	return c.ConsoleWrite("VERBOSE", message)
}

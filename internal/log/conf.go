package log

import (
	"encoding/json"
	"fmt"
	"os"
)

// LogConf is a middleware that stores the log configuration.
// Maintains the data that it needs for Logger to reconstruct itself.
type LogConf struct {
	LogLevel  LogLevel `json:"log_level"`
	FilePath  string   `json:"file_path"`
	FormatStr string   `json:"format_str"`
}

// ConfRead reads the configuration from `/tmp/<name>.json` and returns a LogConf instance.
func ConfRead(name string) (LogConf, error) {
	conf := LogConf{}
	confFile, err := os.ReadFile(fmt.Sprintf("/tmp/%s.json", name))
	if err != nil {
		return LogConf{}, fmt.Errorf("Couldn't read conf file: %s\n", err)
	}
	_ = json.Unmarshal(confFile, &conf)
	return conf, nil
}

// Write is used to write a configuration to `/tmp/<name>.json`.
func (c *LogConf) Write(name string) error {
	logFile, err := os.OpenFile(fmt.Sprintf("/tmp/%s.json", name), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("Failed to open config: %s", err)
	}
	byteConf, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return fmt.Errorf("Failed to jsonify config: %s", err)
	}
	_, err = logFile.Write(byteConf)
	if err != nil {
		return fmt.Errorf("Failed to write config: %s", err)
	}
	return nil
}

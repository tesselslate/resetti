package log

import (
	"encoding/json"
	"fmt"
	"os"
)

type LogConf struct {
	LogLevel  LogLevel `json:"log_level"`
	FilePath  string   `json:"file_path"`
	FormatStr string   `json:"format_str"`
}

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

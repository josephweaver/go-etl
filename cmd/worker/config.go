package main

import "fmt"

type Config struct {
	LogDir        string
	TmpDir        string
	DataDir       string
	ControllerURL string
}

func (c Config) Validate() error {
	if c.LogDir == "" {
		return fmt.Errorf("log dir is required")
	}

	if c.TmpDir == "" {
		return fmt.Errorf("tmp dir is required")
	}

	if c.DataDir == "" {
		return fmt.Errorf("data dir is required")
	}

	if c.ControllerURL == "" {
		return fmt.Errorf("controller url is required")
	}

	return nil
}

func defaultConfig() Config {
	return Config{
		LogDir:        ".run/logs",
		TmpDir:        ".run/tmp",
		DataDir:       ".run/data",
		ControllerURL: "https://controller.local",
	}
}

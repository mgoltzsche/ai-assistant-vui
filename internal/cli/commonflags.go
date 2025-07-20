package cli

import (
	"flag"
	"fmt"
	"log/slog"
)

func addLogLevelFlag(flags *flag.FlagSet) {
	flags.Var(logLevelFlag("INFO"), "log-level", "set the log level")
}

type logLevelFlag string

func (logLevelFlag) Set(s string) error {
	var level slog.Level

	switch s {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		return fmt.Errorf("unsupported log level %q provided. supported log levels are DEBUG, INFO, WARN, ERROR", s)
	}

	slog.SetLogLoggerLevel(level)

	return nil
}

func (f logLevelFlag) String() string {
	return string(f)
}

func (f logLevelFlag) Type() string {
	return "LEVEL"
}

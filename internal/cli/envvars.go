package cli

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

func ParseFlagsWithEnvVars(flags *flag.FlagSet, envVarPrefix string) {
	addLogLevelFlag(flags)

	supportedEnvVars := map[string]struct{}{}
	flags.VisitAll(func(f *flag.Flag) {
		envVarName := envVarPrefix + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		f.Usage = fmt.Sprintf("%s (%s)", f.Usage, envVarName)
		supportedEnvVars[envVarName] = struct{}{}
		if envVarValue := os.Getenv(envVarName); envVarValue != "" {
			f.DefValue = envVarValue
			err := f.Value.Set(envVarValue)
			if err != nil {
				flags.Usage()
				slog.Error(fmt.Sprintf("invalid environment variable %s value provided: %s", envVarValue, err))
				os.Exit(1)
			}
		}
	})

	err := flags.Parse(os.Args[1:])
	if err != nil {
		flags.Usage()
		slog.Error(err.Error())
		os.Exit(1)
	}

	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, envVarPrefix) {
			kv := strings.SplitN(entry, "=", 2)
			if _, ok := supportedEnvVars[kv[0]]; !ok {
				flags.Usage()
				slog.Error(fmt.Sprintf("unsupported environment variable provided: %s", kv[0]))
				os.Exit(1)
			}
		}
	}
}

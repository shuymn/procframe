package config

import "fmt"

// EnvLookup is compatible with os.LookupEnv.
type EnvLookup func(string) (string, bool)

// ApplyEnv looks up envName and, if set, applies it via setter.
func ApplyEnv(lookup EnvLookup, envName string, setter func(string) error) error {
	if envName == "" {
		return nil
	}
	if lookup == nil {
		return fmt.Errorf("env lookup is nil")
	}
	if setter == nil {
		return fmt.Errorf("env setter is nil")
	}

	raw, ok := lookup(envName)
	if !ok {
		return nil
	}
	if err := setter(raw); err != nil {
		return fmt.Errorf("apply env %s: %w", envName, err)
	}
	return nil
}

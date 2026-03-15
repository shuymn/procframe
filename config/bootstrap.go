package config

import (
	"fmt"
	"strings"
)

// ReservedConfigFlag is the flag name reserved for specifying the config file path.
const ReservedConfigFlag = "config"

// BootstrapSpec describes a bootstrap flag accepted by ParseBootstrapArgs.
// The flag name must be kebab-case without the leading "--".
type BootstrapSpec struct {
	Flag string
}

// BootstrapResult holds parsed bootstrap values and remaining procedure args.
type BootstrapResult struct {
	ConfigPath string
	Values     map[string]string
	Rest       []string
}

// ParseBootstrapArgs parses bootstrap flags from the argv prefix.
//
// Only consecutive leading bootstrap flags are consumed. Parsing stops at the
// first non-bootstrap token, and the remaining slice is returned in Rest.
//
// Supported forms:
//   - --config <path>
//   - --config=<path>
//   - --<bootstrap-flag> <value>
//   - --<bootstrap-flag>=<value>
func ParseBootstrapArgs(argv []string, specs []*BootstrapSpec) (*BootstrapResult, error) {
	allowed, err := buildAllowedBootstrapFlags(specs)
	if err != nil {
		return nil, err
	}
	out := &BootstrapResult{
		Values: make(map[string]string, len(specs)),
	}

	for i := 0; i < len(argv); i++ {
		flagName, value, consumed, recognized, err := splitBootstrapArg(argv, i, allowed)
		if err != nil {
			return nil, err
		}
		if !recognized {
			out.Rest = append([]string{}, argv[i:]...)
			return out, nil
		}

		if err := applyBootstrapValue(out, flagName, value); err != nil {
			return nil, err
		}
		i += consumed
	}

	return out, nil
}

func buildAllowedBootstrapFlags(specs []*BootstrapSpec) (map[string]struct{}, error) {
	allowed := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if spec == nil {
			return nil, fmt.Errorf("bootstrap spec must not be nil")
		}
		if err := validateBootstrapFlag(spec.Flag); err != nil {
			return nil, err
		}
		allowed[spec.Flag] = struct{}{}
	}
	return allowed, nil
}

func applyBootstrapValue(out *BootstrapResult, flagName, value string) error {
	if flagName == ReservedConfigFlag {
		if out.ConfigPath != "" {
			return fmt.Errorf("--config specified multiple times")
		}
		out.ConfigPath = value
		return nil
	}
	if _, dup := out.Values[flagName]; dup {
		return fmt.Errorf("--%s specified multiple times", flagName)
	}
	out.Values[flagName] = value
	return nil
}

func splitBootstrapArg(
	argv []string,
	i int,
	allowed map[string]struct{},
) (flagName, value string, consumed int, recognized bool, err error) {
	arg := argv[i]
	if !strings.HasPrefix(arg, "--") || arg == "--" {
		return "", "", 0, false, nil
	}

	nameValue := strings.TrimPrefix(arg, "--")
	name := nameValue
	hasInlineValue := false
	if idx := strings.IndexByte(nameValue, '='); idx >= 0 {
		name = nameValue[:idx]
		value = nameValue[idx+1:]
		hasInlineValue = true
	}

	if name != ReservedConfigFlag {
		if _, ok := allowed[name]; !ok {
			return "", "", 0, false, nil
		}
	}

	if hasInlineValue {
		return name, value, 0, true, nil
	}
	if i+1 >= len(argv) {
		return "", "", 0, false, fmt.Errorf("--%s requires a value", name)
	}
	if looksLikeFlagToken(argv[i+1]) {
		return "", "", 0, false, fmt.Errorf("--%s requires a value", name)
	}

	return name, argv[i+1], 1, true, nil
}

func looksLikeFlagToken(arg string) bool {
	return strings.HasPrefix(arg, "--")
}

func validateBootstrapFlag(name string) error {
	if name == "" {
		return fmt.Errorf("bootstrap flag name must not be empty")
	}
	if strings.HasPrefix(name, "--") {
		return fmt.Errorf("bootstrap flag %q must not include -- prefix", name)
	}
	if strings.Contains(name, "=") {
		return fmt.Errorf("bootstrap flag %q must not include '='", name)
	}
	return nil
}

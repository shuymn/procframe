package config

import (
	"fmt"
	"os"
	"reflect"
)

// Load creates and populates a config of type T from argv.
//
// T can be:
//   - A proto config message implementing SpecProvider (single config)
//   - A struct embedding one or more *ProtoConfig types (composite config)
//
// When multiple configs are embedded, env var and bootstrap flag conflicts
// are detected and --config is disallowed.
func Load[T any](argv []string) (*T, []string, error) {
	cfg := new(T)
	rest, err := loadInto(cfg, argv)
	if err != nil {
		return nil, nil, err
	}
	return cfg, rest, nil
}

func loadInto[T any](cfg *T, argv []string) ([]string, error) {
	specs, err := collectSpecs(cfg)
	if err != nil {
		return nil, err
	}

	err = detectConflicts(specs)
	if err != nil {
		return nil, err
	}

	err = applyAllDefaults(specs)
	if err != nil {
		return nil, err
	}

	bootstrapResult, err := parseUnifiedBootstrap(argv, specs)
	if err != nil {
		return nil, err
	}

	err = applyConfigFile(specs, bootstrapResult.ConfigPath)
	if err != nil {
		return nil, err
	}

	err = applyAllEnv(specs)
	if err != nil {
		return nil, err
	}

	err = applyAllBootstrap(specs, bootstrapResult.Values)
	if err != nil {
		return nil, err
	}

	err = validateAllRequired(specs)
	if err != nil {
		return nil, err
	}

	return bootstrapResult.Rest, nil
}

func applyAllDefaults(specs []*Spec) error {
	for _, spec := range specs {
		if err := spec.ApplyDefaults(); err != nil {
			return fmt.Errorf("apply defaults: %w", err)
		}
	}
	return nil
}

func applyConfigFile(specs []*Spec, configPath string) error {
	if configPath == "" {
		return nil
	}
	if len(specs) > 1 {
		return fmt.Errorf("--config is not supported with composite configs")
	}
	if err := specs[0].ApplyConfigFile(configPath); err != nil {
		return fmt.Errorf("load config file %q: %w", configPath, err)
	}
	return nil
}

func applyAllEnv(specs []*Spec) error {
	for _, spec := range specs {
		if err := spec.ApplyEnv(os.LookupEnv); err != nil {
			return fmt.Errorf("apply env: %w", err)
		}
	}
	return nil
}

func applyAllBootstrap(specs []*Spec, values map[string]string) error {
	for _, spec := range specs {
		if err := spec.ApplyBootstrap(values); err != nil {
			return fmt.Errorf("apply bootstrap: %w", err)
		}
	}
	return nil
}

func validateAllRequired(specs []*Spec) error {
	for _, spec := range specs {
		if err := spec.ValidateRequired(); err != nil {
			return err
		}
	}
	return nil
}

// collectSpecs discovers SpecProvider implementations from T.
// If *T itself implements SpecProvider, it returns a single spec.
// Otherwise it walks T's struct fields for embedded SpecProvider pointers.
func collectSpecs[T any](cfg *T) ([]*Spec, error) {
	// Check if *T directly implements SpecProvider.
	if p, ok := any(cfg).(SpecProvider); ok {
		return []*Spec{p.ConfigSpec()}, nil
	}

	rv := reflect.ValueOf(cfg).Elem()
	rt := rv.Type()
	if rt.Kind() != reflect.Struct {
		return nil, fmt.Errorf("config type %s must be a struct or implement SpecProvider", rt)
	}

	var specs []*Spec
	for i := range rt.NumField() {
		field := rt.Field(i)
		fv := rv.Field(i)

		if !field.Anonymous {
			continue
		}
		if field.Type.Kind() != reflect.Pointer {
			continue
		}

		// Initialize nil pointer fields.
		if fv.IsNil() {
			fv.Set(reflect.New(field.Type.Elem()))
		}

		if p, ok := fv.Interface().(SpecProvider); ok {
			specs = append(specs, p.ConfigSpec())
		}
	}

	if len(specs) == 0 {
		return nil, fmt.Errorf("config type %s has no embedded SpecProvider fields", rt)
	}

	return specs, nil
}

// detectConflicts checks for env var and bootstrap flag name collisions
// across multiple specs.
func detectConflicts(specs []*Spec) error {
	if len(specs) <= 1 {
		return nil
	}

	envSeen := make(map[string]string)
	flagSeen := make(map[string]struct{})

	for _, spec := range specs {
		for envName, fieldPath := range spec.EnvNames {
			if prev, dup := envSeen[envName]; dup {
				return fmt.Errorf(
					"env var %q conflict: %s and %s",
					envName, prev, fieldPath,
				)
			}
			envSeen[envName] = fieldPath
		}
		for _, bs := range spec.BootstrapSpecs {
			if _, dup := flagSeen[bs.Flag]; dup {
				return fmt.Errorf(
					"bootstrap flag --%s conflict: declared by multiple configs",
					bs.Flag,
				)
			}
			flagSeen[bs.Flag] = struct{}{}
		}
	}

	return nil
}

// parseUnifiedBootstrap merges all bootstrap specs and parses argv once.
func parseUnifiedBootstrap(argv []string, specs []*Spec) (*BootstrapResult, error) {
	var allSpecs []*BootstrapSpec
	for _, spec := range specs {
		allSpecs = append(allSpecs, spec.BootstrapSpecs...)
	}
	return ParseBootstrapArgs(argv, allSpecs)
}

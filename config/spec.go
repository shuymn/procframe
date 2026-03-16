package config

// Spec holds config loading metadata and operations.
// Generated ConfigSpec() methods return this with closures capturing
// the config instance and its presence tracker.
//
// All function fields must be non-nil; generated code guarantees this.
type Spec struct {
	EnvNames       map[string]string // env var name -> field path
	BootstrapSpecs []*BootstrapSpec

	ApplyDefaults    func() error
	ApplyConfigFile  func(path string) error
	ApplyEnv         func(lookup EnvLookup) error
	ApplyBootstrap   func(values map[string]string) error
	ValidateRequired func() error
}

// SpecProvider is implemented by generated config messages.
type SpecProvider interface {
	ConfigSpec() *Spec
}

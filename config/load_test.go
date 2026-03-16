package config_test

import (
	"strings"
	"testing"

	"github.com/shuymn/procframe/config"
)

// MockConfig is a manually-implemented SpecProvider for testing.
type MockConfig struct {
	Name    string
	Level   int
	present mockConfigPresence
}

type mockConfigPresence struct {
	Name bool
}

func (c *MockConfig) ConfigSpec() *config.Spec {
	return &config.Spec{
		EnvNames: map[string]string{
			"TEST_NAME":  "MockConfig.name",
			"TEST_LEVEL": "MockConfig.level",
		},
		BootstrapSpecs: []*config.BootstrapSpec{
			{Flag: "name"},
		},

		ApplyDefaults: func() error {
			c.Name = "default"
			c.Level = 1
			return nil
		},
		ApplyConfigFile: func(_ string) error {
			c.Name = "from-file"
			c.present.Name = true
			return nil
		},
		ApplyEnv: func(lookup config.EnvLookup) error {
			if v, ok := lookup("TEST_NAME"); ok {
				c.Name = v
				c.present.Name = true
			}
			if v, ok := lookup("TEST_LEVEL"); ok {
				if v == "2" {
					c.Level = 2
				}
			}
			return nil
		},
		ApplyBootstrap: func(values map[string]string) error {
			if v, ok := values["name"]; ok {
				c.Name = v
				c.present.Name = true
			}
			return nil
		},
		ValidateRequired: func() error {
			return config.ValidateRequired("MockConfig.name", c.present.Name)
		},
	}
}

// MockConfig2 is a second SpecProvider for composite testing.
type MockConfig2 struct {
	Port    int
	Debug   bool
	present mockConfig2Presence
}

type mockConfig2Presence struct {
	Port bool
}

func (c *MockConfig2) ConfigSpec() *config.Spec {
	return &config.Spec{
		EnvNames: map[string]string{
			"TEST2_PORT":  "MockConfig2.port",
			"TEST2_DEBUG": "MockConfig2.debug",
		},
		BootstrapSpecs: []*config.BootstrapSpec{
			{Flag: "port"},
			{Flag: "debug"},
		},

		ApplyDefaults: func() error {
			c.Port = 8080
			return nil
		},
		ApplyConfigFile: func(_ string) error {
			return nil
		},
		ApplyEnv: func(lookup config.EnvLookup) error {
			if v, ok := lookup("TEST2_PORT"); ok {
				if v == "9090" {
					c.Port = 9090
					c.present.Port = true
				}
			}
			if v, ok := lookup("TEST2_DEBUG"); ok {
				c.Debug = v == "true"
			}
			return nil
		},
		ApplyBootstrap: func(values map[string]string) error {
			if v, ok := values["port"]; ok {
				if v == "3000" {
					c.Port = 3000
					c.present.Port = true
				}
			}
			if _, ok := values["debug"]; ok {
				c.Debug = true
			}
			return nil
		},
		ValidateRequired: func() error {
			return config.ValidateRequired("MockConfig2.port", c.present.Port)
		},
	}
}

func TestLoadSingleConfig(t *testing.T) {
	t.Run("applies defaults and bootstrap", func(t *testing.T) {
		t.Parallel()

		cfg, rest, err := config.Load[MockConfig]([]string{"--name", "from-cli", "echo", "run"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Name != "from-cli" {
			t.Fatalf("want name=from-cli, got %q", cfg.Name)
		}
		if cfg.Level != 1 {
			t.Fatalf("want level=1 (default), got %d", cfg.Level)
		}
		if len(rest) != 2 || rest[0] != "echo" || rest[1] != "run" {
			t.Fatalf("unexpected rest args: %v", rest)
		}
	})

	t.Run("applies config file for single config", func(t *testing.T) {
		t.Parallel()

		cfg, rest, err := config.Load[MockConfig]([]string{"--config", "test.json", "echo"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Name != "from-file" {
			t.Fatalf("want name=from-file, got %q", cfg.Name)
		}
		if len(rest) != 1 || rest[0] != "echo" {
			t.Fatalf("unexpected rest args: %v", rest)
		}
	})

	t.Run("applies env", func(t *testing.T) {
		t.Setenv("TEST_NAME", "from-env")
		t.Setenv("TEST_LEVEL", "2")

		cfg, _, err := config.Load[MockConfig]([]string{"echo"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Name != "from-env" {
			t.Fatalf("want name=from-env, got %q", cfg.Name)
		}
		if cfg.Level != 2 {
			t.Fatalf("want level=2, got %d", cfg.Level)
		}
	})

	t.Run("validates required fields", func(t *testing.T) {
		t.Parallel()

		_, _, err := config.Load[MockConfig]([]string{"echo"})
		if err == nil {
			t.Fatal("expected required validation error")
		}
		if !strings.Contains(err.Error(), "MockConfig.name") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

type CompositeConfig struct {
	*MockConfig
	*MockConfig2
}

func TestLoadCompositeConfig(t *testing.T) {
	t.Parallel()

	t.Run("applies defaults and bootstrap for both configs", func(t *testing.T) {
		t.Parallel()

		cfg, rest, err := config.Load[CompositeConfig]([]string{"--name", "cli-name", "--port", "3000", "echo"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Name != "cli-name" {
			t.Fatalf("want name=cli-name, got %q", cfg.Name)
		}
		if cfg.Port != 3000 {
			t.Fatalf("want port=3000, got %d", cfg.Port)
		}
		if len(rest) != 1 || rest[0] != "echo" {
			t.Fatalf("unexpected rest args: %v", rest)
		}
	})

	t.Run("initializes nil pointer fields", func(t *testing.T) {
		t.Parallel()

		cfg, _, err := config.Load[CompositeConfig]([]string{"--name", "test", "--port", "3000", "echo"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.MockConfig == nil {
			t.Fatal("MockConfig should be initialized")
		}
		if cfg.MockConfig2 == nil {
			t.Fatal("MockConfig2 should be initialized")
		}
	})

	t.Run("rejects --config with composite config", func(t *testing.T) {
		t.Parallel()

		_, _, err := config.Load[CompositeConfig]([]string{"--config", "test.json", "echo"})
		if err == nil {
			t.Fatal("expected error for --config with composite config")
		}
		if !strings.Contains(err.Error(), "--config is not supported with composite configs") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("validates required across all configs", func(t *testing.T) {
		t.Parallel()

		_, _, err := config.Load[CompositeConfig]([]string{"echo"})
		if err == nil {
			t.Fatal("expected required validation error")
		}
	})
}

// MockConflictEnvConfig has same env as MockConfig.
type MockConflictEnvConfig struct {
	Value string
}

func (c *MockConflictEnvConfig) ConfigSpec() *config.Spec {
	return &config.Spec{
		EnvNames: map[string]string{
			"TEST_NAME": "MockConflictEnvConfig.value",
		},
		BootstrapSpecs:   []*config.BootstrapSpec{},
		ApplyDefaults:    func() error { return nil },
		ApplyConfigFile:  func(string) error { return nil },
		ApplyEnv:         func(config.EnvLookup) error { return nil },
		ApplyBootstrap:   func(map[string]string) error { return nil },
		ValidateRequired: func() error { return nil },
	}
}

type EnvConflictComposite struct {
	*MockConfig
	*MockConflictEnvConfig
}

func TestLoadEnvConflictDetection(t *testing.T) {
	t.Parallel()

	_, _, err := config.Load[EnvConflictComposite]([]string{"echo"})
	if err == nil {
		t.Fatal("expected env conflict error")
	}
	if !strings.Contains(err.Error(), "TEST_NAME") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// MockConflictBootstrapConfig has same bootstrap flag as MockConfig.
type MockConflictBootstrapConfig struct {
	Value string
}

func (c *MockConflictBootstrapConfig) ConfigSpec() *config.Spec {
	return &config.Spec{
		EnvNames: map[string]string{},
		BootstrapSpecs: []*config.BootstrapSpec{
			{Flag: "name"},
		},
		ApplyDefaults:    func() error { return nil },
		ApplyConfigFile:  func(string) error { return nil },
		ApplyEnv:         func(config.EnvLookup) error { return nil },
		ApplyBootstrap:   func(map[string]string) error { return nil },
		ValidateRequired: func() error { return nil },
	}
}

type BootstrapConflictComposite struct {
	*MockConfig
	*MockConflictBootstrapConfig
}

func TestLoadBootstrapConflictDetection(t *testing.T) {
	t.Parallel()

	_, _, err := config.Load[BootstrapConflictComposite]([]string{"echo"})
	if err == nil {
		t.Fatal("expected bootstrap conflict error")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadNonStructType(t *testing.T) {
	t.Parallel()

	_, _, err := config.Load[string]([]string{"echo"})
	if err == nil {
		t.Fatal("expected error for non-struct type")
	}
	if !strings.Contains(err.Error(), "must be a struct") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type NoProviderStruct struct {
	Name string
}

func TestLoadNoProviderStruct(t *testing.T) {
	t.Parallel()

	_, _, err := config.Load[NoProviderStruct]([]string{"echo"})
	if err == nil {
		t.Fatal("expected error for struct without SpecProvider")
	}
	if !strings.Contains(err.Error(), "no embedded SpecProvider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadBootstrapArgsDistribution(t *testing.T) {
	t.Parallel()

	cfg, rest, err := config.Load[CompositeConfig]([]string{
		"--name", "from-cli",
		"--port", "3000",
		"--debug", "true",
		"echo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "from-cli" {
		t.Fatalf("want name=from-cli, got %q", cfg.Name)
	}
	if cfg.Port != 3000 {
		t.Fatalf("want port=3000, got %d", cfg.Port)
	}
	if !cfg.Debug {
		t.Fatal("want debug=true")
	}
	if len(rest) != 1 || rest[0] != "echo" {
		t.Fatalf("unexpected rest args: %v", rest)
	}
}

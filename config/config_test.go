package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shuymn/procframe/config"
	testv1 "github.com/shuymn/procframe/internal/gen/test/v1"
)

func TestParseBootstrapArgs(t *testing.T) {
	t.Parallel()

	t.Run("parses bootstrap prefix and returns remaining args", func(t *testing.T) {
		t.Parallel()

		specs := []*config.BootstrapSpec{
			{Flag: "message"},
			{Flag: "count"},
		}
		argv := []string{"--config", "cfg.json", "--message=from-cli", "--count", "4", "echo", "run"}

		got, err := config.ParseBootstrapArgs(argv, specs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ConfigPath != "cfg.json" {
			t.Fatalf("want config path cfg.json, got %q", got.ConfigPath)
		}
		if got.Values["message"] != "from-cli" {
			t.Fatalf("want message=from-cli, got %q", got.Values["message"])
		}
		if got.Values["count"] != "4" {
			t.Fatalf("want count=4, got %q", got.Values["count"])
		}
		if len(got.Rest) != 2 || got.Rest[0] != "echo" || got.Rest[1] != "run" {
			t.Fatalf("unexpected rest args: %v", got.Rest)
		}
	})

	t.Run("stops at first non-bootstrap token", func(t *testing.T) {
		t.Parallel()

		specs := []*config.BootstrapSpec{
			{Flag: "message"},
		}
		argv := []string{"--unknown", "value", "--message", "ignored", "echo"}

		got, err := config.ParseBootstrapArgs(argv, specs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ConfigPath != "" {
			t.Fatalf("want empty config path, got %q", got.ConfigPath)
		}
		if len(got.Values) != 0 {
			t.Fatalf("want no parsed bootstrap values, got: %v", got.Values)
		}
		if len(got.Rest) != len(argv) {
			t.Fatalf("want all args untouched, got %v", got.Rest)
		}
	})

	t.Run("duplicate --config returns error", func(t *testing.T) {
		t.Parallel()

		_, err := config.ParseBootstrapArgs(
			[]string{"--config", "a.json", "--config=b.json", "echo"},
			nil,
		)
		if err == nil {
			t.Fatal("expected duplicate --config error")
		}
		if !strings.Contains(err.Error(), "--config specified multiple times") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("nil bootstrap spec returns error", func(t *testing.T) {
		t.Parallel()

		_, err := config.ParseBootstrapArgs(nil, []*config.BootstrapSpec{nil})
		if err == nil {
			t.Fatal("expected nil bootstrap spec error")
		}
		if !strings.Contains(err.Error(), "bootstrap spec must not be nil") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("duplicate bootstrap flag returns error", func(t *testing.T) {
		t.Parallel()

		_, err := config.ParseBootstrapArgs(
			[]string{"--log-level", "info", "--log-level", "debug"},
			[]*config.BootstrapSpec{{Flag: "log-level"}},
		)
		if err == nil {
			t.Fatal("expected duplicate bootstrap flag error")
		}
		if !strings.Contains(err.Error(), "--log-level specified multiple times") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing value returns error", func(t *testing.T) {
		t.Parallel()

		_, err := config.ParseBootstrapArgs(
			[]string{"--count"},
			[]*config.BootstrapSpec{{Flag: "count"}},
		)
		if err == nil {
			t.Fatal("expected missing value error")
		}
		if !strings.Contains(err.Error(), "--count requires a value") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("known flag followed by flag-like token returns error", func(t *testing.T) {
		t.Parallel()

		_, err := config.ParseBootstrapArgs(
			[]string{"--count", "--proc-flag", "echo", "run"},
			[]*config.BootstrapSpec{{Flag: "count"}},
		)
		if err == nil {
			t.Fatal("expected missing value error")
		}
		if !strings.Contains(err.Error(), "--count requires a value") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("config flag followed by flag-like token returns error", func(t *testing.T) {
		t.Parallel()

		_, err := config.ParseBootstrapArgs(
			[]string{"--config", "--proc-flag", "echo", "run"},
			[]*config.BootstrapSpec{{Flag: "count"}},
		)
		if err == nil {
			t.Fatal("expected missing value error")
		}
		if !strings.Contains(err.Error(), "--config requires a value") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestMergeJSONFileAndEnvAndBootstrapPrecedence(t *testing.T) {
	cfg := &testv1.EchoRequest{
		Message:   "default-message",
		Count:     1,
		Uppercase: false,
	}

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"message":"file-message","count":0,"uppercase":true}`), 0o600); err != nil {
		t.Fatalf("write temp config file: %v", err)
	}

	if _, err := config.MergeJSONFile(path, cfg); err != nil {
		t.Fatalf("merge JSON file: %v", err)
	}
	if cfg.Message != "file-message" {
		t.Fatalf("want message from file, got %q", cfg.Message)
	}
	if cfg.Count != 0 {
		t.Fatalf("want count overridden to 0 by file, got %d", cfg.Count)
	}
	if !cfg.Uppercase {
		t.Fatal("want uppercase=true from file")
	}

	t.Setenv("PF_MESSAGE", "env-message")
	t.Setenv("PF_COUNT", "7")
	if err := config.ApplyEnv(os.LookupEnv, "PF_MESSAGE", func(raw string) error {
		cfg.Message = raw
		return nil
	}); err != nil {
		t.Fatalf("apply message env: %v", err)
	}
	if err := config.ApplyEnv(os.LookupEnv, "PF_COUNT", func(raw string) error {
		n, err := config.ParseInt32(raw)
		if err != nil {
			return err
		}
		cfg.Count = n
		return nil
	}); err != nil {
		t.Fatalf("apply count env: %v", err)
	}
	if cfg.Message != "env-message" {
		t.Fatalf("want message from env, got %q", cfg.Message)
	}
	if cfg.Count != 7 {
		t.Fatalf("want count from env=7, got %d", cfg.Count)
	}

	parsed, err := config.ParseBootstrapArgs(
		[]string{"--message", "bootstrap-message", "--count=9", "echo", "run"},
		[]*config.BootstrapSpec{
			{Flag: "message"},
			{Flag: "count"},
		},
	)
	if err != nil {
		t.Fatalf("parse bootstrap args: %v", err)
	}
	cfg.Message = parsed.Values["message"]
	n, err := config.ParseInt32(parsed.Values["count"])
	if err != nil {
		t.Fatalf("parse bootstrap count: %v", err)
	}
	cfg.Count = n

	if cfg.Message != "bootstrap-message" {
		t.Fatalf("want message from bootstrap, got %q", cfg.Message)
	}
	if cfg.Count != 9 {
		t.Fatalf("want count from bootstrap=9, got %d", cfg.Count)
	}
	if len(parsed.Rest) != 2 || parsed.Rest[0] != "echo" || parsed.Rest[1] != "run" {
		t.Fatalf("unexpected rest args: %v", parsed.Rest)
	}
}

func TestMergeJSONFileRequiresPath(t *testing.T) {
	t.Parallel()

	cfg := &testv1.EchoRequest{}
	presentFields, err := config.MergeJSONFile("", cfg)
	if err == nil {
		t.Fatal("expected empty path error")
	}
	if presentFields != nil {
		t.Fatalf("want nil present fields on error, got %v", presentFields)
	}
	if !strings.Contains(err.Error(), "read config file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyEnv(t *testing.T) {
	t.Parallel()

	t.Run("missing env is no-op", func(t *testing.T) {
		t.Parallel()
		called := false
		err := config.ApplyEnv(
			func(string) (string, bool) { return "", false },
			"UNSET",
			func(string) error {
				called = true
				return nil
			},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Fatal("setter should not be called")
		}
	})

	t.Run("setter error is wrapped", func(t *testing.T) {
		t.Parallel()
		baseErr := errors.New("boom")
		err := config.ApplyEnv(
			func(string) (string, bool) { return "v", true },
			"SET_ENV",
			func(string) error { return baseErr },
		)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "apply env SET_ENV") {
			t.Fatalf("unexpected error: %v", err)
		}
		if !errors.Is(err, baseErr) {
			t.Fatalf("expected wrapped base error, got: %v", err)
		}
	})
}

func TestParseScalarsAndEnum(t *testing.T) {
	t.Parallel()

	if v, err := config.ParseBool("true"); err != nil || !v {
		t.Fatalf("ParseBool(true) = (%v, %v)", v, err)
	}
	if v, err := config.ParseInt32("-12"); err != nil || v != -12 {
		t.Fatalf("ParseInt32(-12) = (%v, %v)", v, err)
	}
	if v, err := config.ParseUint64("42"); err != nil || v != 42 {
		t.Fatalf("ParseUint64(42) = (%v, %v)", v, err)
	}
	if v, err := config.ParseFloat32("3.5"); err != nil || v < 3.4 || v > 3.6 {
		t.Fatalf("ParseFloat32(3.5) = (%v, %v)", v, err)
	}
	if _, err := config.ParseInt32("nope"); err == nil {
		t.Fatal("ParseInt32(nope) should fail")
	}

	enum, err := config.ParseEnum("open", []*config.EnumMapping{
		{Name: "open", Number: 1},
		{Name: "closed", Number: 2},
	}, "State")
	if err != nil {
		t.Fatalf("ParseEnum(open): %v", err)
	}
	if enum != 1 {
		t.Fatalf("want enum=1, got %d", enum)
	}
	if _, err := config.ParseEnum("draft", []*config.EnumMapping{
		{Name: "open", Number: 1},
	}, "State"); err == nil {
		t.Fatal("ParseEnum(draft) should fail")
	}
}

func TestRequiredAndRedaction(t *testing.T) {
	t.Parallel()

	if err := config.ValidateRequired("token", false); err == nil {
		t.Fatal("expected required error")
	}
	if err := config.ValidateRequired("count", true); err != nil {
		t.Fatalf("unexpected required error: %v", err)
	}
	if err := config.ValidateRequired("zero", true); err != nil {
		t.Fatalf("explicit zero value should be accepted when present: %v", err)
	}

	if got := config.RedactIfSet("secret", config.RedactedPlaceholder); got != config.RedactedPlaceholder {
		t.Fatalf("unexpected redacted value: %q", got)
	}
	if got := config.RedactIfSet("", config.RedactedPlaceholder); got != "" {
		t.Fatalf("empty value should stay empty, got: %q", got)
	}

	masked := config.RedactBytes([]byte("token"))
	if string(masked) != config.RedactedPlaceholder {
		t.Fatalf("unexpected masked bytes: %q", string(masked))
	}
	if got := config.RedactBytes(nil); got != nil {
		t.Fatalf("nil bytes should stay nil, got %v", got)
	}
}

func TestGeneratedConfigLoad(t *testing.T) {
	t.Run("applies defaults file env bootstrap in order", func(t *testing.T) {
		t.Setenv("SERVICE_NAME", "from-env")
		t.Setenv("LOG_LEVEL", "debug")
		t.Setenv("API_TOKEN", "env-token")

		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(`{"serviceName":"from-file","timeoutSec":0}`), 0o600); err != nil {
			t.Fatalf("write config file: %v", err)
		}

		cfg, rest, err := config.Load[testv1.RuntimeConfig]([]string{
			"--config", path,
			"--timeout-sec", "12",
			"repo", "pr", "list",
		})
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}

		if cfg.ServiceName != "from-env" {
			t.Fatalf("want service name from env, got %q", cfg.ServiceName)
		}
		if cfg.LogLevel != testv1.LogLevel_LOG_LEVEL_DEBUG {
			t.Fatalf("want log level from env, got %v", cfg.LogLevel)
		}
		if cfg.TimeoutSec != 12 {
			t.Fatalf("want timeout from bootstrap, got %d", cfg.TimeoutSec)
		}
		if cfg.ApiToken != "env-token" {
			t.Fatalf("want api token from env, got %q", cfg.ApiToken)
		}
		if strings.Join(rest, " ") != "repo pr list" {
			t.Fatalf("unexpected rest args: %v", rest)
		}
	})

	t.Run("file can override default with zero value", func(t *testing.T) {
		t.Setenv("API_TOKEN", "env-token")

		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(`{"timeoutSec":0}`), 0o600); err != nil {
			t.Fatalf("write config file: %v", err)
		}

		cfg, rest, err := config.Load[testv1.RuntimeConfig]([]string{"--config", path, "echo", "run"})
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}
		if cfg.TimeoutSec != 0 {
			t.Fatalf("want timeout overridden to 0 by file, got %d", cfg.TimeoutSec)
		}
		if strings.Join(rest, " ") != "echo run" {
			t.Fatalf("unexpected rest args: %v", rest)
		}
	})

	t.Run("file accepts protobuf field names", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(
			path,
			[]byte(`{"service_name":"from-file","timeout_sec":0,"api_token":""}`),
			0o600,
		); err != nil {
			t.Fatalf("write config file: %v", err)
		}

		cfg, rest, err := config.Load[testv1.RuntimeConfig]([]string{"--config", path, "echo", "run"})
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}
		if cfg.ServiceName != "from-file" {
			t.Fatalf("want service name from protobuf field name, got %q", cfg.ServiceName)
		}
		if cfg.TimeoutSec != 0 {
			t.Fatalf("want timeout overridden to 0 by protobuf field name, got %d", cfg.TimeoutSec)
		}
		if cfg.ApiToken != "" {
			t.Fatalf("want required api token from protobuf field name, got %q", cfg.ApiToken)
		}
		if strings.Join(rest, " ") != "echo run" {
			t.Fatalf("unexpected rest args: %v", rest)
		}
	})

	t.Run("file accepts enum alias", func(t *testing.T) {
		t.Setenv("API_TOKEN", "env-token")

		path := writeConfigFile(t, `{"logLevel":"debug"}`)
		cfg, _, err := config.Load[testv1.RuntimeConfig]([]string{"--config", path, "echo"})
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}
		if cfg.LogLevel != testv1.LogLevel_LOG_LEVEL_DEBUG {
			t.Fatalf("want log level from alias, got %v", cfg.LogLevel)
		}
	})

	t.Run("file accepts proto enum name", func(t *testing.T) {
		t.Setenv("API_TOKEN", "env-token")

		path := writeConfigFile(t, `{"logLevel":"LOG_LEVEL_DEBUG"}`)
		cfg, _, err := config.Load[testv1.RuntimeConfig]([]string{"--config", path, "echo"})
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}
		if cfg.LogLevel != testv1.LogLevel_LOG_LEVEL_DEBUG {
			t.Fatalf("want log level from proto enum name, got %v", cfg.LogLevel)
		}
	})

	t.Run("file accepts enum number", func(t *testing.T) {
		t.Setenv("API_TOKEN", "env-token")

		path := writeConfigFile(t, `{"logLevel":2}`)
		cfg, _, err := config.Load[testv1.RuntimeConfig]([]string{"--config", path, "echo"})
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}
		if cfg.LogLevel != testv1.LogLevel_LOG_LEVEL_DEBUG {
			t.Fatalf("want log level from enum number, got %v", cfg.LogLevel)
		}
	})

	t.Run("file accepts enum numeric string", func(t *testing.T) {
		t.Setenv("API_TOKEN", "env-token")

		path := writeConfigFile(t, `{"logLevel":"2"}`)
		cfg, _, err := config.Load[testv1.RuntimeConfig]([]string{"--config", path, "echo"})
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}
		if cfg.LogLevel != testv1.LogLevel_LOG_LEVEL_DEBUG {
			t.Fatalf("want log level from enum numeric string, got %v", cfg.LogLevel)
		}
	})

	t.Run("file rejects invalid enum alias with allowed values", func(t *testing.T) {
		t.Setenv("API_TOKEN", "env-token")

		path := writeConfigFile(t, `{"logLevel":"trace"}`)
		_, _, err := config.Load[testv1.RuntimeConfig]([]string{"--config", path, "echo"})
		if err == nil {
			t.Fatal("expected invalid enum error")
		}
		if !strings.Contains(err.Error(), `invalid LogLevel value "trace"`) {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(err.Error(), "info, debug") {
			t.Fatalf("want allowed values in error, got: %v", err)
		}
	})

	t.Run("missing required field returns error", func(t *testing.T) {
		_, _, err := config.Load[testv1.RuntimeConfig]([]string{"echo", "run"})
		if err == nil {
			t.Fatal("expected missing required field error")
		}
		if !strings.Contains(err.Error(), "RuntimeConfig.api_token") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("explicit empty required value is accepted", func(t *testing.T) {
		t.Setenv("API_TOKEN", "")

		cfg, rest, err := config.Load[testv1.RuntimeConfig]([]string{"echo", "run"})
		if err != nil {
			t.Fatalf("Load returned error: %v", err)
		}
		if cfg.ApiToken != "" {
			t.Fatalf("want explicit empty required value preserved, got %q", cfg.ApiToken)
		}
		if strings.Join(rest, " ") != "echo run" {
			t.Fatalf("unexpected rest args: %v", rest)
		}
	})

	t.Run("duplicate bootstrap flag returns error", func(t *testing.T) {
		t.Setenv("API_TOKEN", "env-token")
		_, _, err := config.Load[testv1.RuntimeConfig]([]string{
			"--timeout-sec", "10",
			"--timeout-sec", "20",
			"echo", "run",
		})
		if err == nil {
			t.Fatal("expected duplicate bootstrap flag error")
		}
		if !strings.Contains(err.Error(), "--timeout-sec specified multiple times") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("format masks secret fields for pointer", func(t *testing.T) {
		cfg := testv1.RuntimeConfig{
			ServiceName: "svc",
			LogLevel:    testv1.LogLevel_LOG_LEVEL_INFO,
			TimeoutSec:  30,
			ApiToken:    "secret-token",
			SecretPort:  9000,
		}

		rendered := fmt.Sprintf("%+v", &cfg)
		if strings.Contains(rendered, "secret-token") {
			t.Fatalf("secret string leaked in formatted output: %s", rendered)
		}
		if strings.Contains(rendered, "9000") {
			t.Fatalf("secret numeric field leaked in formatted output: %s", rendered)
		}
		if !strings.Contains(rendered, config.RedactedPlaceholder) {
			t.Fatalf("want redaction placeholder in formatted output: %s", rendered)
		}
		if !strings.Contains(rendered, `"serviceName"`) || !strings.Contains(rendered, `"svc"`) {
			t.Fatalf("non-secret field missing from formatted output: %s", rendered)
		}
		if cfg.ApiToken != "secret-token" {
			t.Fatalf("original config mutated: %q", cfg.ApiToken)
		}
	})

	t.Run("nil config pointer formats as nil", func(t *testing.T) {
		var cfg *testv1.RuntimeConfig
		if rendered := fmt.Sprintf("%v", cfg); rendered != "<nil>" {
			t.Fatalf("want <nil>, got %q", rendered)
		}
	})

	t.Run("secret env parse errors are redacted", func(t *testing.T) {
		t.Setenv("API_TOKEN", "env-token")
		t.Setenv("SECRET_PORT", "not-a-number")

		_, _, err := config.Load[testv1.RuntimeConfig]([]string{"echo", "run"})
		if err == nil {
			t.Fatal("expected secret env parse error")
		}
		if !strings.Contains(err.Error(), "invalid secret value") {
			t.Fatalf("want invalid secret value error, got %v", err)
		}
		if strings.Contains(err.Error(), "not-a-number") {
			t.Fatalf("secret value leaked in error: %v", err)
		}
	})

	t.Run("secret bootstrap parse errors are redacted", func(t *testing.T) {
		t.Setenv("API_TOKEN", "env-token")

		_, _, err := config.Load[testv1.RuntimeConfig]([]string{
			"--secret-port", "not-a-number",
			"echo", "run",
		})
		if err == nil {
			t.Fatal("expected secret bootstrap parse error")
		}
		if !strings.Contains(err.Error(), "invalid secret value") {
			t.Fatalf("want invalid secret value error, got %v", err)
		}
		if strings.Contains(err.Error(), "not-a-number") {
			t.Fatalf("secret value leaked in error: %v", err)
		}
	})

	t.Run("secret file parse errors are redacted", func(t *testing.T) {
		t.Setenv("API_TOKEN", "env-token")

		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(`{"secret_port":"not-a-number"}`), 0o600); err != nil {
			t.Fatalf("write config file: %v", err)
		}

		_, _, err := config.Load[testv1.RuntimeConfig]([]string{
			"--config", path,
			"echo", "run",
		})
		if err == nil {
			t.Fatal("expected secret file parse error")
		}
		if strings.Contains(err.Error(), "not-a-number") {
			t.Fatalf("secret value leaked in error: %v", err)
		}
		if !strings.Contains(err.Error(), config.RedactedPlaceholder) {
			t.Fatalf("want redaction placeholder in error: %v", err)
		}
	})

	t.Run("known bootstrap flag followed by flag-like token returns error", func(t *testing.T) {
		t.Setenv("API_TOKEN", "env-token")

		_, _, err := config.Load[testv1.RuntimeConfig]([]string{
			"--timeout-sec", "--proc-flag",
			"echo", "run",
		})
		if err == nil {
			t.Fatal("expected missing value error")
		}
		if !strings.Contains(err.Error(), "--timeout-sec requires a value") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestLoad_EmptyArgv verifies that config.Load with empty argv
// does not panic and either returns a valid config or an error.
func TestLoad_EmptyArgv(t *testing.T) {
	t.Setenv("API_TOKEN", "env-token")

	cfg, rest, err := config.Load[testv1.RuntimeConfig]([]string{})
	if err != nil {
		// Error is acceptable (e.g. missing required fields).
		return
	}
	if cfg == nil {
		t.Fatal("Load returned nil config without error")
	}
	if rest == nil {
		// nil rest is acceptable for empty argv.
		return
	}
	if len(rest) != 0 {
		t.Fatalf("want empty rest for empty argv, got %v", rest)
	}
}

// TestLoad_NilArgv verifies that config.Load with nil argv
// does not panic.
func TestLoad_NilArgv(t *testing.T) {
	t.Setenv("API_TOKEN", "env-token")

	cfg, _, err := config.Load[testv1.RuntimeConfig](nil)
	if err != nil {
		return // Error is acceptable.
	}
	if cfg == nil {
		t.Fatal("Load returned nil config without error")
	}
}

// TestMergeJSONFile_NilDestination verifies that MergeJSONFile
// with a nil destination returns an error and does not panic.
func TestMergeJSONFile_NilDestination(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, err := config.MergeJSONFile(path, nil)
	if err == nil {
		t.Fatal("expected error for nil destination")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Fatalf("want nil-related error, got: %v", err)
	}
}

// TestApplyEnv_NilLookup verifies that ApplyEnv with nil
// lookup function returns an error.
func TestApplyEnv_NilLookup(t *testing.T) {
	t.Parallel()

	err := config.ApplyEnv(nil, "KEY", func(string) error { return nil })
	if err == nil {
		t.Fatal("expected error for nil lookup")
	}
}

// TestApplyEnv_NilSetter verifies that ApplyEnv with nil
// setter function returns an error.
func TestApplyEnv_NilSetter(t *testing.T) {
	t.Parallel()

	err := config.ApplyEnv(os.LookupEnv, "KEY", nil)
	if err == nil {
		t.Fatal("expected error for nil setter")
	}
}

// TestApplyEnv_EmptyEnvName verifies that ApplyEnv with
// empty env name is a no-op.
func TestApplyEnv_EmptyEnvName(t *testing.T) {
	t.Parallel()

	called := false
	err := config.ApplyEnv(os.LookupEnv, "", func(string) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("setter should not be called for empty env name")
	}
}

// TestParseBootstrapArgs_EmptyFlagName verifies that a BootstrapSpec
// with an empty flag name is rejected.
func TestParseBootstrapArgs_EmptyFlagName(t *testing.T) {
	t.Parallel()

	_, err := config.ParseBootstrapArgs(
		[]string{"--", "value"},
		[]*config.BootstrapSpec{{Flag: ""}},
	)
	if err == nil {
		t.Fatal("expected error for empty flag name")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLoad_ConfigFilePathTraversal verifies that path traversal
// sequences in config file paths are handled by the OS and do not cause
// unexpected behavior. Since config file paths are user-provided argv
// values, the user already has filesystem access; this test verifies
// that the error is clean and non-panicking.
func TestLoad_ConfigFilePathTraversal(t *testing.T) {
	t.Setenv("API_TOKEN", "env-token")

	// Attempt path traversal.
	traversalPaths := []string{
		"../../../etc/passwd",
		"/etc/passwd",
		"./../../nonexistent",
		string([]byte{'.', '.', '/', '.', '.', '/', 0x00, 'e', 't', 'c'}),
	}

	for _, path := range traversalPaths {
		_, _, err := config.Load[testv1.RuntimeConfig]([]string{
			"--config", path, "echo",
		})
		if err == nil {
			// If the file doesn't exist or is not valid JSON, Load should
			// return an error. If it happens to be valid JSON (e.g. /etc/passwd
			// doesn't usually exist as valid JSON), that's the OS's job.
			t.Logf("path %q: Load returned no error (file may not exist or may be valid)", path)
			continue
		}
		// Verify error is clean and doesn't panic.
		if strings.Contains(err.Error(), "panic") {
			t.Fatalf("path %q: error contains panic: %v", path, err)
		}
	}
}

// TestLoad_MalformedJSONConfigFile verifies that a config file with
// malformed JSON returns a clean error.
func TestLoad_MalformedJSONConfigFile(t *testing.T) {
	t.Setenv("API_TOKEN", "env-token")

	malformedCases := []struct {
		name    string
		content string
	}{
		{"empty_file", ""},
		{"not_json", "this is not json"},
		{"truncated_json", `{"serviceName": "test`},
		{"array_not_object", `["a", "b", "c"]`},
		{"nested_bomb", `{"serviceName": ` + strings.Repeat(`{"a":`, 100) + `"x"` + strings.Repeat(`}`, 100) + `}`},
	}

	for _, tc := range malformedCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			_, _, err := config.Load[testv1.RuntimeConfig]([]string{
				"--config", path, "echo",
			})
			if err == nil {
				t.Fatalf("expected error for malformed JSON: %s", tc.name)
			}
		})
	}
}

// TestParseBootstrapArgs_FlagNameInjection verifies that bootstrap flag names
// with special characters are rejected.
func TestParseBootstrapArgs_FlagNameInjection(t *testing.T) {
	t.Parallel()

	injectionCases := []struct {
		name string
		flag string
	}{
		{"with_prefix", "--flag"},
		{"with_equals", "flag=value"},
		{"empty", ""},
	}

	for _, tc := range injectionCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := config.ParseBootstrapArgs(
				[]string{"--" + tc.flag, "value"},
				[]*config.BootstrapSpec{{Flag: tc.flag}},
			)
			if err == nil {
				t.Fatalf("expected error for injected flag %q", tc.flag)
			}
		})
	}
}

// checkNoInternalExposure verifies an error message doesn't leak Go runtime
// internals, file paths, or stack traces.
func checkNoInternalExposure(t *testing.T, msg string) {
	t.Helper()
	sensitive := []string{
		".go:",
		"goroutine ",
		"runtime.",
		"panic:",
		"/Users/",
		"/home/",
		"github.com/",
	}
	for _, s := range sensitive {
		if strings.Contains(msg, s) {
			t.Errorf("error message leaks internal detail %q: %s", s, msg)
		}
	}
}

// writeConfigFile is a helper that creates a temporary config file.
func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}

// TestLoad_ConfigFieldCaseCollision verifies that a config file
// containing both camelCase and snake_case forms of the same field is
// detected as a duplicate.
func TestLoad_ConfigFieldCaseCollision(t *testing.T) {
	// Not parallel: uses t.Setenv.
	t.Setenv("API_TOKEN", "env-token")

	path := writeConfigFile(t, `{
		"serviceName": "camel",
		"service_name": "snake"
	}`)

	_, _, err := config.Load[testv1.RuntimeConfig]([]string{"--config", path, "echo"})
	if err == nil {
		t.Fatal("expected error for duplicate camelCase/snake_case field")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected 'duplicate' in error, got: %v", err)
	}
	checkNoInternalExposure(t, err.Error())
}

// TestLoad_ConfigUnknownFieldRejection verifies that config files
// with unknown fields are rejected (DiscardUnknown: false).
func TestLoad_ConfigUnknownFieldRejection(t *testing.T) {
	// Not parallel: uses t.Setenv.
	t.Setenv("API_TOKEN", "env-token")

	path := writeConfigFile(t, `{
		"serviceName": "test",
		"unknownField": "should-be-rejected"
	}`)

	_, _, err := config.Load[testv1.RuntimeConfig]([]string{"--config", path, "echo"})
	if err == nil {
		t.Fatal("expected error for unknown field in config file")
	}
	checkNoInternalExposure(t, err.Error())
}

// TestMergeJSONFile_SecretRedactionInErrors verifies that secret field
// values (apiToken) are redacted in error messages when MergeJSONFile
// encounters a protojson unmarshal error.
func TestMergeJSONFile_SecretRedactionInErrors(t *testing.T) {
	t.Parallel()

	t.Run("secret_not_in_type_error", func(t *testing.T) {
		t.Parallel()

		// Config with valid apiToken but type error on timeoutSec.
		// The secret should not appear in the error message.
		path := writeConfigFile(t, `{
			"apiToken": "super-secret-token-12345",
			"timeoutSec": "not-a-number"
		}`)

		dst := &testv1.RuntimeConfig{}
		_, err := config.MergeJSONFile(path, dst, "apiToken")
		if err == nil {
			t.Fatal("expected error for type mismatch")
		}
		if strings.Contains(err.Error(), "super-secret-token-12345") {
			t.Error("VULNERABLE: secret apiToken value leaked in error message")
		}
		checkNoInternalExposure(t, err.Error())
	})

	t.Run("secret_in_own_type_error", func(t *testing.T) {
		t.Parallel()

		// Config where the SECRET field itself has a type error.
		// protojson error will contain the value; redaction must replace it.
		path := writeConfigFile(t, `{
			"apiToken": 99999,
			"serviceName": "test"
		}`)

		dst := &testv1.RuntimeConfig{}
		_, err := config.MergeJSONFile(path, dst, "apiToken")
		if err == nil {
			t.Fatal("expected error for type mismatch on secret field")
		}
		// "99999" is the secret value. It should be redacted.
		if strings.Contains(err.Error(), "99999") {
			t.Error("VULNERABLE: secret apiToken value leaked in error message")
		}
		checkNoInternalExposure(t, err.Error())
	})

	t.Run("unknown_field_does_not_leak_secret", func(t *testing.T) {
		t.Parallel()

		// Config with an unknown field and a secret.
		path := writeConfigFile(t, `{
			"apiToken": "classified-key-abc",
			"hackField": "attack"
		}`)

		dst := &testv1.RuntimeConfig{}
		_, err := config.MergeJSONFile(path, dst, "apiToken")
		if err == nil {
			t.Fatal("expected error for unknown field")
		}
		if strings.Contains(err.Error(), "classified-key-abc") {
			t.Error("VULNERABLE: secret apiToken value leaked in unknown field error")
		}
		checkNoInternalExposure(t, err.Error())
	})
}

// TestLoad_PartialFailureReturnsNil verifies that config
// loading errors leave the pipeline in a clean state and don't
// produce partial results.
func TestLoad_PartialFailureReturnsNil(t *testing.T) {
	t.Run("file_error_returns_nil_config", func(t *testing.T) {
		// Not parallel: uses t.Setenv.
		t.Setenv("API_TOKEN", "env-token")

		// Config file with type error — Load should return nil config.
		path := writeConfigFile(t, `{
			"serviceName": 12345
		}`)

		cfg, _, err := config.Load[testv1.RuntimeConfig]([]string{
			"--config", path, "echo",
		})
		if err == nil {
			t.Fatal("expected error for type mismatch in config file")
		}
		if cfg != nil {
			t.Error("expected nil config on load error, got non-nil")
		}
	})

	t.Run("env_error_returns_nil_config", func(t *testing.T) {
		// Not parallel: uses t.Setenv.
		// Set an invalid value for an integer field via env var.
		t.Setenv("API_TOKEN", "valid-token")
		t.Setenv("TIMEOUT_SEC", "not-an-integer")

		cfg, _, err := config.Load[testv1.RuntimeConfig]([]string{"echo"})
		if err == nil {
			t.Fatal("expected error for invalid env var value")
		}
		if cfg != nil {
			t.Error("expected nil config on env parse error, got non-nil")
		}
		checkNoInternalExposure(t, err.Error())
	})
}

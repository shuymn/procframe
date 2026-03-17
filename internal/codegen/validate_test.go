package codegen

import (
	"strings"
	"testing"
)

func TestValidateDuplicatePaths(t *testing.T) {
	t.Parallel()

	t.Run("no duplicates", func(t *testing.T) {
		t.Parallel()
		services := []serviceInfo{
			{
				Path: []string{"repo"},
				Methods: []methodInfo{
					{Path: []string{"list"}, CLI: true, FullName: "/svc/List"},
					{Path: []string{"get"}, CLI: true, FullName: "/svc/Get"},
				},
			},
		}
		if err := validateDuplicatePaths(services); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("duplicate detected", func(t *testing.T) {
		t.Parallel()
		services := []serviceInfo{
			{
				Path: []string{"repo"},
				Methods: []methodInfo{
					{Path: []string{"list"}, CLI: true, FullName: "/svc1/List"},
				},
			},
			{
				Path: []string{"repo"},
				Methods: []methodInfo{
					{Path: []string{"list"}, CLI: true, FullName: "/svc2/List"},
				},
			},
		}
		err := validateDuplicatePaths(services)
		if err == nil {
			t.Fatal("expected error for duplicate paths")
		}
		if !strings.Contains(err.Error(), "duplicate CLI path") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("cli=false skipped", func(t *testing.T) {
		t.Parallel()
		services := []serviceInfo{
			{
				Path: []string{"repo"},
				Methods: []methodInfo{
					{Path: []string{"list"}, CLI: true, FullName: "/svc1/List"},
					{Path: []string{"list"}, CLI: false, FullName: "/svc2/List"},
				},
			},
		}
		if err := validateDuplicatePaths(services); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestValidateDuplicatePaths_EdgeCases probes the path
// validation with edge cases.
func TestValidateDuplicatePaths_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty_services", func(t *testing.T) {
		t.Parallel()
		err := validateDuplicatePaths(nil)
		if err != nil {
			t.Fatalf("unexpected error for nil services: %v", err)
		}
	})

	t.Run("empty_path_segments", func(t *testing.T) {
		t.Parallel()
		services := []serviceInfo{
			{
				Methods: []methodInfo{
					{CLI: true, Path: []string{}, FullName: "/a"},
					{CLI: true, Path: []string{}, FullName: "/b"},
				},
			},
		}
		// Both methods have empty path -> both resolve to ""
		// -> duplicate detected.
		err := validateDuplicatePaths(services)
		if err == nil {
			t.Fatal("expected duplicate error for empty paths")
		}
		checkNoInternalLeak(t, err.Error())
	})
}

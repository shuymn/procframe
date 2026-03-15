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

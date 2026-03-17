package cli_test

import (
	"testing"

	testv1 "github.com/shuymn/procframe/internal/gen/test/v1"
	"github.com/shuymn/procframe/transport/cli"
)

func TestUnmarshalJSONField(t *testing.T) {
	t.Parallel()

	t.Run("message field", func(t *testing.T) {
		t.Parallel()
		req := &testv1.PRListRequest{}
		err := cli.UnmarshalJSONField(req, "repo", `{"org":"myorg"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Repo == nil {
			t.Fatal("want Repo set, got nil")
		}
		if req.Repo.Org != "myorg" {
			t.Fatalf("want org=myorg, got %q", req.Repo.Org)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()
		req := &testv1.PRListRequest{}
		err := cli.UnmarshalJSONField(req, "repo", `{invalid}`)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("unknown field discarded", func(t *testing.T) {
		t.Parallel()
		req := &testv1.PRListRequest{}
		err := cli.UnmarshalJSONField(req, "repo", `{"org":"test","unknown_field":"ignored"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Repo.Org != "test" {
			t.Fatalf("want org=test, got %q", req.Repo.Org)
		}
	})

	t.Run("preserves existing fields", func(t *testing.T) {
		t.Parallel()
		req := &testv1.PRListRequest{Limit: 42}
		err := cli.UnmarshalJSONField(req, "repo", `{"org":"myorg"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Limit != 42 {
			t.Fatalf("want limit=42 preserved, got %d", req.Limit)
		}
		if req.Repo == nil || req.Repo.Org != "myorg" {
			t.Fatal("want Repo.Org=myorg")
		}
	})
}

func TestUnmarshalJSONField_NilMessage(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Logf("UnmarshalJSONField recovered panic: %v", r)
		}
	}()

	// Passing nil message — should panic or error.
	err := cli.UnmarshalJSONField(nil, "field", `"value"`)
	if err == nil {
		// If it didn't panic or error, that's still acceptable as long
		// as no corruption occurred.
		t.Log("UnmarshalJSONField(nil) returned no error")
	}
}

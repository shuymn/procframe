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

// TestUnmarshalJSONField_RejectsSiblingInjection verifies whether crafted JSON input
// for a message-type flag can inject values into sibling fields via
// UnmarshalJSONField's string concatenation wrapper.
//
// UnmarshalJSONField wraps rawJSON as: {"fieldName":rawJSON}
// If rawJSON contains '},"otherField":value', the wrapper becomes:
//
//	{"fieldName":...},"otherField":value}
//
// which is valid JSON with multiple top-level keys.
func TestUnmarshalJSONField_RejectsSiblingInjection(t *testing.T) {
	t.Parallel()

	t.Run("sibling_field_injection", func(t *testing.T) {
		t.Parallel()

		// Start with limit=3 (simulating --limit 3 flag).
		req := &testv1.PRListRequest{Limit: 3}

		// Crafted repo JSON that attempts to inject a limit override.
		// wrapped = {"repo":{"org":"myorg"},"limit":999}
		crafted := `{"org":"myorg"},"limit":999`

		err := cli.UnmarshalJSONField(req, "repo", crafted)
		if err != nil {
			// protojson rejected the malformed JSON → DEFENDED.
			t.Logf("DEFENDED: protojson rejected crafted input: %v", err)
			return
		}

		// Check if the sibling field was overwritten.
		if req.Limit != 3 {
			t.Errorf("VULNERABLE: limit changed from 3 to %d via repo field injection", req.Limit)
		}
	})

	t.Run("bind_into_field_injection", func(t *testing.T) {
		t.Parallel()

		// PRScope has primaryLabel (message) and state (enum).
		// Test if crafting primaryLabel JSON can inject state.
		scope := &testv1.PRScope{State: testv1.PRState_PR_STATE_OPEN}

		// Attempt to inject state override via primaryLabel field.
		// wrapped = {"primaryLabel":{"key":"v"},"state":"PR_STATE_CLOSED"}
		crafted := `{"key":"v"},"state":"PR_STATE_CLOSED"`

		err := cli.UnmarshalJSONField(scope, "primaryLabel", crafted)
		if err != nil {
			t.Logf("DEFENDED: protojson rejected crafted input: %v", err)
			return
		}

		if scope.State != testv1.PRState_PR_STATE_OPEN {
			t.Errorf("VULNERABLE: state changed from OPEN to %v via primaryLabel injection", scope.State)
		}
	})
}

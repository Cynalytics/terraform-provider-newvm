package provider

import (
	"context"
	"testing"

	"unithost-terraform/internal/newvm"
)

func TestFlattenOrderMetadata_EmptyIsTypedEmptyMap(t *testing.T) {
	t.Parallel()

	meta, diags := flattenOrderMetadata(context.Background(), []newvm.NewVmOrderMetaData{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics errors: %v", diags.Errors())
	}

	if meta.IsNull() {
		t.Fatalf("expected metadata to be an empty map, got null")
	}

	if meta.IsUnknown() {
		t.Fatalf("expected metadata to be known, got unknown")
	}

	if got := len(meta.Elements()); got != 0 {
		t.Fatalf("expected 0 metadata elements, got %d", got)
	}
}

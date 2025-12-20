package croupier

import (
	"context"
	"testing"
)

func TestInvoker_validatePayload(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:        "127.0.0.1:19090",
		TimeoutSeconds: 1,
		Insecure:       true,
	})

	impl, ok := i.(*invoker)
	if !ok {
		t.Fatalf("expected *invoker, got %T", i)
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []any{"name"},
	}

	if err := impl.SetSchema("f1", schema); err != nil {
		t.Fatalf("SetSchema: %v", err)
	}

	if _, err := impl.Invoke(context.Background(), "f1", `{}`, InvokeOptions{}); err == nil {
		t.Fatalf("expected validation error")
	}

	if _, err := impl.Invoke(context.Background(), "f1", `{"name":"alice"}`, InvokeOptions{}); err == nil {
		t.Fatalf("expected connect error (no server), got nil")
	}
}

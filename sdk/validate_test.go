package sdk

import "testing"

func TestExclusiveMinimum(t *testing.T){
    s := map[string]any{"type":"number","minimum":5.0,"exclusiveMinimum":true}
    if err := ValidateJSON(s, 5.0); err == nil { t.Fatalf("expected exclusiveMinimum violation") }
    if err := ValidateJSON(s, 5.1); err != nil { t.Fatalf("expected ok, got %v", err) }
}

func TestFormats(t *testing.T){
    se := map[string]any{"type":"string","format":"email"}
    if err := ValidateJSON(se, "a@b.com"); err != nil { t.Fatalf("email ok, got %v", err) }
    if err := ValidateJSON(se, "not"); err == nil { t.Fatalf("expected email invalid") }
    su := map[string]any{"type":"string","format":"uuid"}
    if err := ValidateJSON(su, "550e8400-e29b-41d4-a716-446655440000"); err != nil { t.Fatalf("uuid ok, got %v", err) }
    if err := ValidateJSON(su, "550e8400"); err == nil { t.Fatalf("expected uuid invalid") }
}

func TestAnyOfOneOf(t *testing.T){
    s := map[string]any{"anyOf": []any{ map[string]any{"type":"string"}, map[string]any{"type":"number"} }}
    if err := ValidateJSON(s, "ok"); err != nil { t.Fatalf("anyOf string ok, got %v", err) }
    if err := ValidateJSON(s, 1.0); err != nil { t.Fatalf("anyOf number ok, got %v", err) }
    so := map[string]any{"oneOf": []any{ map[string]any{"type":"string"}, map[string]any{"type":"number"} }}
    if err := ValidateJSON(so, "ok"); err != nil { t.Fatalf("oneOf string ok, got %v", err) }
    if err := ValidateJSON(so, 1.0); err != nil { t.Fatalf("oneOf number ok, got %v", err) }
    if err := ValidateJSON(so, nil); err == nil { t.Fatalf("expected oneOf invalid for nil") }
}

func TestTupleItems(t *testing.T){
    s := map[string]any{"type":"array","items": []any{ map[string]any{"type":"string"}, map[string]any{"type":"number"} }, "minItems":2, "maxItems":2}
    if err := ValidateJSON(s, []any{"x", 2.0}); err != nil { t.Fatalf("tuple ok, got %v", err) }
    if err := ValidateJSON(s, []any{"x", "y"}); err == nil { t.Fatalf("expected tuple invalid") }
}


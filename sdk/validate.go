package sdk

import (
    "errors"
    "fmt"
    "regexp"
)

// ValidateJSON validates a decoded JSON value against a minimal JSON-Schema subset.
// Supported keywords: type, properties, required, additionalProperties, items,
// minItems, maxItems, minLength, maxLength, minimum, maximum, enum, pattern.
// Schema is expected to be a map[string]any (e.g., from descriptor.Params).
func ValidateJSON(schema map[string]any, v any) error {
    return validate(schema, v, "$")
}

func validate(s map[string]any, v any, path string) error {
    // type
    if t, ok := getString(s, "type"); ok {
        switch t {
        case "object":
            m, ok := v.(map[string]any)
            if !ok && v != nil { return fmt.Errorf("%s: expected object", path) }
            // required
            if req, ok := getStringSlice(s, "required"); ok {
                for _, k := range req {
                    if m == nil { return fmt.Errorf("%s: required %s missing", path, k) }
                    if _, ok := m[k]; !ok { return fmt.Errorf("%s: required %s missing", path, k) }
                }
            }
            // properties
            props, _ := getMap(s, "properties")
            for k, sub := range props {
                if mv, exists := m[k]; exists {
                    if subm, ok := sub.(map[string]any); ok {
                        if err := validate(subm, mv, path+"."+k); err != nil { return err }
                    }
                }
            }
            // additionalProperties
            if addp, ok := s["additionalProperties"]; ok {
                if b, ok := addp.(bool); ok && b == false {
                    // ensure no unknown keys beyond properties
                    for k := range m {
                        if _, ok := props[k]; !ok {
                            return fmt.Errorf("%s: additional property %s not allowed", path, k)
                        }
                    }
                }
            }
        case "array":
            arr, ok := toSlice(v)
            if !ok && v != nil { return fmt.Errorf("%s: expected array", path) }
            if minI, ok := getNumber(s, "minItems"); ok {
                if len(arr) < int(minI) { return fmt.Errorf("%s: minItems %v", path, int(minI)) }
            }
            if maxI, ok := getNumber(s, "maxItems"); ok {
                if len(arr) > int(maxI) { return fmt.Errorf("%s: maxItems %v", path, int(maxI)) }
            }
            if items, ok := getMap(s, "items"); ok {
                for i, it := range arr {
                    if err := validate(items, it, fmt.Sprintf("%s[%d]", path, i)); err != nil { return err }
                }
            }
        case "string":
            str, ok := v.(string)
            if !ok && v != nil { return fmt.Errorf("%s: expected string", path) }
            if minL, ok := getNumber(s, "minLength"); ok {
                if len(str) < int(minL) { return fmt.Errorf("%s: minLength %v", path, int(minL)) }
            }
            if maxL, ok := getNumber(s, "maxLength"); ok {
                if len(str) > int(maxL) { return fmt.Errorf("%s: maxLength %v", path, int(maxL)) }
            }
            if pat, ok := getString(s, "pattern"); ok && pat != "" {
                re, err := regexp.Compile(pat); if err != nil { return fmt.Errorf("%s: invalid pattern", path) }
                if !re.MatchString(str) { return fmt.Errorf("%s: pattern mismatch", path) }
            }
            if enum, ok := getSlice(s, "enum"); ok {
                if !contains(enum, str) { return fmt.Errorf("%s: not in enum", path) }
            }
        case "number":
            num, ok := toNumber(v)
            if !ok && v != nil { return fmt.Errorf("%s: expected number", path) }
            if min, ok := getFloat(s, "minimum"); ok { if num < min { return fmt.Errorf("%s: minimum %v", path, min) } }
            if max, ok := getFloat(s, "maximum"); ok { if num > max { return fmt.Errorf("%s: maximum %v", path, max) } }
            if enum, ok := getSlice(s, "enum"); ok { if !contains(enum, num) { return fmt.Errorf("%s: not in enum", path) } }
        case "integer":
            num, ok := toNumber(v)
            if !ok && v != nil { return fmt.Errorf("%s: expected integer", path) }
            if num != float64(int64(num)) { return fmt.Errorf("%s: not an integer", path) }
            if min, ok := getFloat(s, "minimum"); ok { if num < min { return fmt.Errorf("%s: minimum %v", path, min) } }
            if max, ok := getFloat(s, "maximum"); ok { if num > max { return fmt.Errorf("%s: maximum %v", path, max) } }
            if enum, ok := getSlice(s, "enum"); ok { if !contains(enum, num) { return fmt.Errorf("%s: not in enum", path) } }
        case "boolean":
            if _, ok := v.(bool); !ok && v != nil { return fmt.Errorf("%s: expected boolean", path) }
        case "null":
            if v != nil { return fmt.Errorf("%s: expected null", path) }
        default:
            return errors.New("unsupported type: "+t)
        }
    }
    return nil
}

// helpers
func getString(m map[string]any, k string) (string, bool) { v, ok := m[k]; if !ok { return "", false }; s, ok := v.(string); return s, ok }
func getMap(m map[string]any, k string) (map[string]any, bool) { v, ok := m[k]; if !ok { return nil, false }; mm, ok := v.(map[string]any); return mm, ok }
func getSlice(m map[string]any, k string) ([]any, bool) { v, ok := m[k]; if !ok { return nil, false }; switch vv := v.(type) { case []any: return vv, true; default: return nil, false } }
func getStringSlice(m map[string]any, k string) ([]string, bool) { v, ok := getSlice(m, k); if !ok { return nil, false }; out := make([]string, 0, len(v)); for _, e := range v { if s, ok := e.(string); ok { out = append(out, s) } }; return out, true }
func getNumber(m map[string]any, k string) (float64, bool) { v, ok := m[k]; if !ok { return 0, false }; switch n := v.(type) { case float64: return n, true; case int: return float64(n), true; case int64: return float64(n), true; default: return 0, false } }
func getFloat(m map[string]any, k string) (float64, bool) { return getNumber(m, k) }
func toSlice(v any) ([]any, bool) { if v == nil { return []any{}, true }; if s, ok := v.([]any); ok { return s, true }; return nil, false }
func toNumber(v any) (float64, bool) { switch n := v.(type) { case float64: return n, true; case int: return float64(n), true; case int64: return float64(n), true; default: return 0, false } }
func contains(arr []any, target any) bool { for _, e := range arr { if equalJSON(e, target) { return true } }; return false }
func equalJSON(a, b any) bool {
    switch av := a.(type) {
    case float64:
        bv, ok := toNumber(b); return ok && av == bv
    default:
        return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
    }
}


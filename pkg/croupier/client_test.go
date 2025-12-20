package croupier

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"
)

func TestClient_buildManifest(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ServiceID:      "svc-1",
			ServiceVersion: "sv1",
			ProviderLang:   "go",
			ProviderSDK:    "croupier-go-sdk",
		},
		handlers:    map[string]FunctionHandler{},
		descriptors: map[string]FunctionDescriptor{},
	}

	c.descriptors["f1"] = FunctionDescriptor{
		ID:        "f1",
		Version:   "1.2.3",
		Category:  "cat",
		Risk:      "low",
		Entity:    "player",
		Operation: "create",
		Enabled:   true,
	}
	c.descriptors["f2"] = FunctionDescriptor{
		ID:      "f2",
		Version: "",
		Enabled: false,
	}

	raw, err := c.buildManifest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded struct {
		Provider struct {
			ID      string `json:"id"`
			Version string `json:"version"`
			Lang    string `json:"lang"`
			SDK     string `json:"sdk"`
		} `json:"provider"`
		Functions []map[string]any `json:"functions"`
	}

	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if decoded.Provider.ID != "svc-1" || decoded.Provider.Version != "sv1" || decoded.Provider.Lang != "go" || decoded.Provider.SDK != "croupier-go-sdk" {
		t.Fatalf("unexpected provider: %+v", decoded.Provider)
	}

	funcByID := map[string]map[string]any{}
	for _, fn := range decoded.Functions {
		id, _ := fn["id"].(string)
		funcByID[id] = fn
	}

	if fn := funcByID["f1"]; fn == nil || fn["version"] != "1.2.3" || fn["entity"] != "player" || fn["operation"] != "create" {
		t.Fatalf("unexpected f1: %+v", fn)
	}
	if _, ok := funcByID["f1"]["enabled"]; !ok {
		t.Fatalf("expected enabled flag to be present for f1")
	}

	if fn := funcByID["f2"]; fn == nil || fn["version"] != "1.0.0" {
		t.Fatalf("unexpected f2: %+v", fn)
	}
	if _, ok := funcByID["f2"]["enabled"]; ok {
		t.Fatalf("expected enabled flag to be omitted for f2 when false")
	}
}

func TestGzipBytes_RoundTrip(t *testing.T) {
	t.Parallel()

	original := []byte(`{"hello":"world"}`)
	compressed, err := gzipBytes(original)
	if err != nil {
		t.Fatalf("gzipBytes: %v", err)
	}

	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer r.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read gzipped: %v", err)
	}
	if !bytes.Equal(out, original) {
		t.Fatalf("round-trip mismatch: got %q want %q", out, original)
	}
}

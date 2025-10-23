package sdk

import (
    "encoding/json"
    "os"
    "strings"
)

// FileDescriptor represents a simple function descriptor file.
type FileDescriptor struct {
    ID      string                 `json:"id"`
    Version string                 `json:"version"`
    Params  map[string]any         `json:"params"`
    Semantics map[string]any       `json:"semantics"`
}

func LoadDescriptor(path string) (*FileDescriptor, error) {
    b, err := os.ReadFile(path)
    if err != nil { return nil, err }
    var d FileDescriptor
    if err := json.Unmarshal(b, &d); err != nil { return nil, err }
    return &d, nil
}

// RegisterFromDescriptor loads a descriptor from file and registers the function with the handler.
func (c *Client) RegisterFromDescriptor(path string, h Handler) error {
    d, err := LoadDescriptor(path)
    if err != nil { return err }
    return c.RegisterFunction(Function{ID: d.ID, Version: d.Version, Schema: d.Params}, h)
}

// RegisterFromDir loads all *.json descriptors under dir and register them using the same handler for demo purpose.
// In real projects you likely map different handlers per function.
func (c *Client) RegisterFromDir(dir string, handlerFactory func(id string) Handler) error {
    entries, err := os.ReadDir(dir)
    if err != nil { return err }
    for _, e := range entries {
        if e.IsDir() { continue }
        if !strings.HasSuffix(e.Name(), ".json") { continue }
        d, err := LoadDescriptor(dir+"/"+e.Name())
        if err != nil { return err }
        if err := c.RegisterFunction(Function{ID: d.ID, Version: d.Version, Schema: d.Params}, handlerFactory(d.ID)); err != nil { return err }
    }
    return nil
}

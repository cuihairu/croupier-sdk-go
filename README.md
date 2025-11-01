# Croupier SDK (Go)

Go SDK for Croupier.

Features
- Connect to Agent (gRPC), host local handlers via FunctionService (JSON codec)
- Register functions to Agent (LocalControlService)
- Minimal JSON Schema validation before handler (type/properties/required/items/enum/limits)
- Simple retry/timeout interceptors (grpc.DialOptions)
- Idempotency key helper

Quick Example
```go
package main

import (
  "context"
  "log"
  sdk "github.com/cuihairu/croupier-sdk-go/sdk"
)

func main(){
  c := sdk.NewClient(sdk.ClientConfig{Addr: "127.0.0.1:19090", ServiceID: "game-1", ServiceVersion: "1.0.0"})
  // register a function handler
  _ = c.RegisterFunction(sdk.Function{ID: "echo", Version: "1.0.0", Schema: map[string]any{
    "type": "object",
    "required": []any{"msg"},
    "properties": map[string]any{
      "msg": map[string]any{"type": "string", "minLength": 1},
    },
  }}, func(ctx context.Context, payload []byte)([]byte,error){
    return payload, nil
  })
  // connect and block until Ctrl+C
  ctx := context.Background()
  if err := c.Serve(ctx); err != nil { log.Fatal(err) }
}
```

Server/Agent Invoker
```go
inv, _ := sdk.NewInvoker(context.Background(), sdk.InvokerConfig{
  Address:  "127.0.0.1:19090", // FunctionService endpoint (Agent or Server)
  Insecure: true,               // use mTLS in production
  Timeout:  3 * time.Second,
  GameID:   "default", Env: "dev",
})
defer inv.Close()
inv.SetSchema("player.ban", map[string]any{"type":"object","required":[]any{"player_id"}})
out, err := inv.Invoke(ctx, "player.ban", []byte(`{"player_id":"u-1"}`))
// job flow:
jobID, _ := inv.StartJob(ctx, "long.task", []byte(`{"x":1}`))
ch, _ := inv.StreamJob(ctx, jobID); for ev := range ch { /* handle */ }
_ = inv.CancelJob(ctx, jobID)
```

Install
```bash
go get github.com/cuihairu/croupier-sdk-go@latest
```

Notes
- API may evolve; JSON stubs are hand-written for lightweight dev use.
- For end-to-end demo, run Croupier Server/Agent and the example above; after Agent registers, Server can route to your local handler.
 - Validation errors return as "payload invalid: ..." from handler entry.

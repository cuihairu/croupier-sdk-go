# 使用示例

本章节提供 Croupier Go SDK 的使用示例。

## 基础示例

### main.go

```go
package main

import (
    "context"
    "encoding/json"
    "log"

    "github.com/cuihairu/croupier/sdks/go/pkg/croupier"
)

func main() {
    config := &croupier.ClientConfig{
        AgentAddr: "localhost:19090",
        GameID:    "demo-game",
        Env:       "development",
        Insecure:  true,
    }

    client := croupier.NewClient(config)

    // 注册函数
    desc := croupier.FunctionDescriptor{
        ID:      "hello.world",
        Version: "0.1.0",
        Enabled: true,
    }

    handler := func(ctx context.Context, payload string) (string, error) {
        return `{"message":"Hello from Go!"}`, nil
    }

    if err := client.RegisterFunction(desc, handler); err != nil {
        log.Fatal(err)
    }

    // 启动服务
    log.Println("Starting server...")
    if err := client.Serve(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

### 运行

```bash
go run main.go
```

## 综合示例

详见 [examples/comprehensive](https://github.com/cuihairu/croupier-sdk-go/tree/main/examples/comprehensive)。

## 更多示例

- [基础示例](https://github.com/cuihairu/croupier-sdk-go/tree/main/examples/basic)
- [综合示例](https://github.com/cuihairu/croupier-sdk-go/tree/main/examples/comprehensive)

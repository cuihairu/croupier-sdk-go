# API 参考

本文档提供 Croupier Go SDK 的完整 API 参考。

## 核心类型

### ClientConfig

客户端配置结构。

```go
type ClientConfig struct {
    // 连接配置
    AgentAddr      string // Agent gRPC 地址
    LocalListen    string // 本地服务器地址
    TimeoutSeconds int    // 连接超时
    Insecure       bool   // 使用不安全的 gRPC

    // 多租户隔离
    GameID         string // 游戏标识符
    Env            string // 环境（dev/staging/prod）
    ServiceID      string // 服务标识符
    ServiceVersion string // 服务版本
    AgentID        string // Agent 标识符

    // TLS（非 insecure 模式）
    CAFile   string // CA 证书
    CertFile string // 客户端证书
    KeyFile  string // 私钥
}
```

### FunctionDescriptor

函数描述符。

```go
type FunctionDescriptor struct {
    ID        string // 函数 ID，如 "player.ban"
    Version   string // 语义化版本，如 "0.1.0"
    Category  string // 分组类别
    Risk      string // "low"|"medium"|"high"
    Entity    string // 实体类型，如 "player"
    Operation string // "create"|"read"|"update"|"delete"
    Enabled   bool   // 是否启用
}
```

### LocalFunctionDescriptor

本地函数描述符。

```go
type LocalFunctionDescriptor struct {
    ID      string // 函数 ID
    Version string // 函数版本
}
```

### Handler

函数处理器类型。

```go
type Handler func(ctx context.Context, payload string) (string, error)
```

## Client 接口

```go
type Client interface {
    // 注册函数
    RegisterFunction(desc FunctionDescriptor, handler Handler) error

    // 启动服务
    Serve(ctx context.Context) error

    // 关闭连接
    Close() error
}
```

## 创建客户端

```go
import "github.com/cuihairu/croupier/sdks/go/pkg/croupier"

config := &croupier.ClientConfig{
    AgentAddr: "localhost:19090",
    GameID:    "my-game",
    Env:       "development",
    Insecure:  true,
}

client := croupier.NewClient(config)
```

## 注册函数

```go
desc := croupier.FunctionDescriptor{
    ID:      "player.ban",
    Version: "0.1.0",
    Enabled: true,
}

handler := func(ctx context.Context, payload string) (string, error) {
    return `{"success":true}`, nil
}

if err := client.RegisterFunction(desc, handler); err != nil {
    log.Fatal(err)
}
```

## 启动服务

```go
ctx := context.Background()
if err := client.Serve(ctx); err != nil {
    log.Fatal(err)
}
```

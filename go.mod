module github.com/cuihairu/croupier/sdks/go

go 1.25.6

require (
	// ============================================================================
	// 版本锁定 - 必须与 croupier-proto/CLAUDE.md 保持一致！
	// protobuf v1.36.11 对应 protoc-gen-go v1.36.11
	// grpc v1.69.0 对应 grpc remote plugin v1.69.0
	// ============================================================================
	google.golang.org/grpc v1.69.0
	google.golang.org/protobuf v1.36.11
)

require github.com/xeipuuv/gojsonschema v1.2.0

require (
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	golang.org/x/net v0.46.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda // indirect
)

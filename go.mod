module github.com/cuihairu/croupier/sdks/go

go 1.25.6

require (
	// NNG/mangos for transport layer
	go.nanomsg.org/mangos/v3 v3.4.2

	// Protobuf for message serialization
	google.golang.org/protobuf v1.36.11
)

// Local replace for main croupier module
replace github.com/cuihairu/croupier => ../croupier

require (
	github.com/cuihairu/croupier v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
	github.com/xeipuuv/gojsonschema v1.2.0
	google.golang.org/grpc v1.78.0
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260202165425-ce8ad4cf556b // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

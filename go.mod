module github.com/envector/rune-go

go 1.26.2

// External dependencies, in implementation order:
//
//   github.com/modelcontextprotocol/go-sdk v1.5.0  — MCP protocol (D2) ✅ Phase A
//   google.golang.org/grpc v1.65.0                  — Vault / envector / embedder clients (Phase 4)
//   google.golang.org/protobuf v1.34.0              — generated stubs (Phase 4)
//   github.com/CryptoLabInc/envector-go-sdk         — envector FHE client (Q4 PR pending)
//
// go 1.25.0 + toolchain pin required by the MCP SDK.

require (
	github.com/CryptoLabInc/envector-go-sdk v0.1.0
	github.com/CryptoLabInc/rune-admin/vault v0.0.0-20260506055025-ad52b6bd549d
	github.com/CryptoLabInc/runed v0.0.0
	github.com/modelcontextprotocol/go-sdk v1.5.0
	google.golang.org/grpc v1.81.0
)

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20260415201107-50325440f8f2.1 // indirect
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/CryptoLabInc/runed => ../runed

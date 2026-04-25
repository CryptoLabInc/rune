module github.com/envector/rune-go

go 1.25.0

// External dependencies to be added as implementation progresses:
//
//   github.com/modelcontextprotocol/go-sdk v1.5.0  — MCP protocol (D2)
//   google.golang.org/grpc v1.65.0                  — Vault / envector / embedder clients
//   google.golang.org/protobuf v1.34.0               — generated stubs
//   github.com/CryptoLabInc/envector-go-sdk          — envector FHE client (Q4 PR pending)
//
// Skeleton stage: stdlib only. No external imports yet.

require github.com/modelcontextprotocol/go-sdk v1.5.0

require (
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

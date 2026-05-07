package mcp

import (
	"encoding/json"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/envector/rune-go/internal/domain"
)

// errorResult shapes a tool failure into a CallToolResult per the rune-mcp
// error contract (spec/components/rune-mcp.md §에러 처리).
//
// The body is JSON-marshalled domain.MakeError output so the agent receives
// {"ok":false,"error":{"code","message","retryable","recovery_hint"}}. We mark
// IsError=true so the SDK forwards the error semantics to the caller.
func errorResult(err error) *sdkmcp.CallToolResult {
	body, _ := json.Marshal(domain.MakeError(err))
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(body)}},
	}
}

// okResult builds a success CallToolResult whose TextContent is the
// JSON-encoded result payload. Out (typed) is also returned by the handler
// so the SDK populates StructuredContent.
//
// Failure to marshal payload (rare — only structurally bad output types) is
// reported as INTERNAL_ERROR rather than panicking.
func okResult(payload any) *sdkmcp.CallToolResult {
	body, err := json.Marshal(payload)
	if err != nil {
		return errorResult(&domain.RuneError{
			Code:    domain.CodeInternal,
			Message: "failed to marshal result: " + err.Error(),
		})
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(body)}},
	}
}

package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/service"
)

// ─────────────────────────────────────────────────────────────────────────────
// Write tools — gated by CheckState (PIPELINE_NOT_READY when not active).
// ─────────────────────────────────────────────────────────────────────────────

func handleCapture(deps *Deps) sdkmcp.ToolHandlerFor[domain.CaptureRequest, domain.CaptureResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in domain.CaptureRequest) (*sdkmcp.CallToolResult, domain.CaptureResponse, error) {
		var zero domain.CaptureResponse
		if err := CheckState(deps.State); err != nil {
			return errorResult(err), zero, nil
		}
		if err := ValidateCaptureRequest(&in); err != nil {
			return errorResult(err), zero, nil
		}
		out, err := deps.Capture.Handle(ctx, &in)
		if err != nil {
			return errorResult(err), zero, nil
		}
		return okResult(out), *out, nil
	}
}

func handleBatchCapture(deps *Deps) sdkmcp.ToolHandlerFor[service.BatchCaptureArgs, service.BatchCaptureResult] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in service.BatchCaptureArgs) (*sdkmcp.CallToolResult, service.BatchCaptureResult, error) {
		var zero service.BatchCaptureResult
		if err := CheckState(deps.State); err != nil {
			return errorResult(err), zero, nil
		}
		out, err := deps.Capture.Batch(ctx, in)
		if err != nil {
			return errorResult(err), zero, nil
		}
		return okResult(out), *out, nil
	}
}

func handleRecall(deps *Deps) sdkmcp.ToolHandlerFor[domain.RecallArgs, domain.RecallResult] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in domain.RecallArgs) (*sdkmcp.CallToolResult, domain.RecallResult, error) {
		var zero domain.RecallResult
		if err := CheckState(deps.State); err != nil {
			return errorResult(err), zero, nil
		}
		if err := ValidateRecallArgs(&in); err != nil {
			return errorResult(err), zero, nil
		}
		out, err := deps.Recall.Handle(ctx, &in)
		if err != nil {
			return errorResult(err), zero, nil
		}
		return okResult(out), *out, nil
	}
}

func handleDeleteCapture(deps *Deps) sdkmcp.ToolHandlerFor[service.DeleteCaptureArgs, service.DeleteCaptureResult] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in service.DeleteCaptureArgs) (*sdkmcp.CallToolResult, service.DeleteCaptureResult, error) {
		var zero service.DeleteCaptureResult
		if err := CheckState(deps.State); err != nil {
			return errorResult(err), zero, nil
		}
		out, err := deps.Lifecycle.DeleteCapture(ctx, in, deps.Capture)
		if err != nil {
			return errorResult(err), zero, nil
		}
		return okResult(out), *out, nil
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Read / diagnostic tools — bypass CheckState. Surface partial state to the
// agent (vault_status, diagnostics) so users can troubleshoot pre-active.
// ─────────────────────────────────────────────────────────────────────────────

func handleCaptureHistory(deps *Deps) sdkmcp.ToolHandlerFor[service.CaptureHistoryArgs, service.CaptureHistoryResult] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in service.CaptureHistoryArgs) (*sdkmcp.CallToolResult, service.CaptureHistoryResult, error) {
		var zero service.CaptureHistoryResult
		out, err := deps.Lifecycle.CaptureHistory(ctx, in)
		if err != nil {
			return errorResult(err), zero, nil
		}
		return okResult(out), *out, nil
	}
}

func handleVaultStatus(deps *Deps) sdkmcp.ToolHandlerFor[emptyArgs, service.VaultStatusResult] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ emptyArgs) (*sdkmcp.CallToolResult, service.VaultStatusResult, error) {
		var zero service.VaultStatusResult
		out, err := deps.Lifecycle.VaultStatus(ctx)
		if err != nil {
			return errorResult(err), zero, nil
		}
		return okResult(out), *out, nil
	}
}

func handleDiagnostics(deps *Deps) sdkmcp.ToolHandlerFor[emptyArgs, service.DiagnosticsResult] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ emptyArgs) (*sdkmcp.CallToolResult, service.DiagnosticsResult, error) {
		out := deps.Lifecycle.Diagnostics(ctx)
		return okResult(out), *out, nil
	}
}

func handleReloadPipelines(deps *Deps) sdkmcp.ToolHandlerFor[emptyArgs, service.ReloadPipelinesResult] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ emptyArgs) (*sdkmcp.CallToolResult, service.ReloadPipelinesResult, error) {
		var zero service.ReloadPipelinesResult
		out, err := deps.Lifecycle.ReloadPipelines(ctx)
		if err != nil {
			return errorResult(err), zero, nil
		}
		return okResult(out), *out, nil
	}
}

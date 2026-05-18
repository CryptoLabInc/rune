package service

import (
	"context"
	"errors"
	"log/slog"

	sdk "github.com/CryptoLabInc/envector-go-sdk"

	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/lifecycle"
)

// We cannot easily rely on tranport interceptor since Insert and Score are streaming gRPC
// Below helpers replicate interceptor: wait for retrigger on a retryable failure then retry once

func insertWithRecovery(ctx context.Context, state *lifecycle.Manager, c envector.Client, req envector.InsertRequest) (*envector.InsertResult, error) {
	res, err := c.Insert(ctx, req)
	if err == nil {
		return res, nil
	}
	if errors.Is(err, sdk.ErrAlreadyExists) {
		slog.Info("capture: insert request_id already committed (idempotent retry)",
			"request_id", req.RequestID)
		return &envector.InsertResult{}, nil
	}
	if state == nil || !isEnvectorRetryable(err) {
		return nil, err
	}

	if !state.WaitForActive(ctx, lifecycle.RecoverTimeout) {
		return nil, err
	}

	res, err = c.Insert(ctx, req)
	if err == nil {
		return res, nil
	}
	if errors.Is(err, sdk.ErrAlreadyExists) {
		slog.Info("capture: insert request_id already committed on retry",
			"request_id", req.RequestID)
		return &envector.InsertResult{}, nil
	}
	return nil, err
}

func scoreWithRecovery(ctx context.Context, state *lifecycle.Manager, c envector.Client, vec []float32) ([][]byte, error) {
	blobs, err := c.Score(ctx, vec)
	if err == nil {
		return blobs, nil
	}
	if state == nil || !isEnvectorRetryable(err) {
		return nil, err
	}
	if !state.WaitForActive(ctx, lifecycle.RecoverTimeout) {
		return nil, err
	}
	return c.Score(ctx, vec)
}

func isEnvectorRetryable(err error) bool {
	var e *envector.Error
	return errors.As(err, &e) && e.Retryable
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
)

func TestIsNormalShutdown(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil — stdin EOF surfaces here (SDK filters io.EOF to nil)", nil, true},
		{"context.Canceled — SIGINT/SIGTERM path", context.Canceled, true},
		{"wrapped context.Canceled", fmt.Errorf("wrap: %w", context.Canceled), true},
		{"raw io.EOF should NOT match — SDK already filters it", io.EOF, false},
		{"context.DeadlineExceeded is a real error in shutdown", context.DeadlineExceeded, false},
		{"arbitrary error", errors.New("transport broken"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNormalShutdown(tc.err); got != tc.want {
				t.Errorf("isNormalShutdown(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

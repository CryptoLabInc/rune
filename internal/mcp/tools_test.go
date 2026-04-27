// Internal-package tests for the panic-guard wrapper around sdkmcp.AddTool.
// (Phase A.5 in-memory smoke is in register_test.go which uses package mcp_test.)

package mcp

import (
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestIsValidToolName(t *testing.T) {
	if !isValidToolName("rune_capture") {
		t.Error("rune_capture should be valid")
	}
	if isValidToolName("") {
		t.Error("empty should be invalid")
	}
	if isValidToolName("rune capture") {
		t.Error("name with space should be invalid")
	}
	if isValidToolName(strings.Repeat("a", 129)) {
		t.Error("name >128 chars should be invalid")
	}
}

func TestMustAddTool_PanicsOnInvalidName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("mustAddTool with invalid name did not panic")
		}
	}()
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "x", Version: "0"}, nil)
	mustAddTool[emptyArgs, emptyArgs](srv, &Deps{}, "rune capture", "test")
}

func TestRegister_AllHardcodedNamesValid(t *testing.T) {
	// Sanity: Register's 8 hardcoded names all pass mustAddTool's check.
	// Catches an accidental typo in tools.go before Phase A.5 integration test runs.
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "x", Version: "0"}, nil)
	if err := Register(srv, &Deps{}); err != nil {
		t.Errorf("Register returned error: %v", err)
	}
}

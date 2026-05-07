package mcp_test

import (
	"bytes"
	"testing"

	"github.com/envector/rune-go/internal/adapters/vault"
	"github.com/envector/rune-go/internal/mcp"
	"github.com/envector/rune-go/internal/service"
)

func newDepsForApply() *mcp.Deps {
	return &mcp.Deps{
		Capture:   service.NewCaptureService(),
		Recall:    service.NewRecallService(),
		Lifecycle: service.NewLifecycleService(),
	}
}

func TestApplyVaultBundle_PropagatesToCapture(t *testing.T) {
	d := newDepsForApply()
	bundle := &vault.Bundle{
		AgentID:   "agent_test",
		AgentDEK:  bytes.Repeat([]byte{0xAB}, 32),
		IndexName: "team-index",
		KeyID:     "key_xyz",
		EncKey:    []byte("non-empty"),
	}

	d.ApplyVaultBundle(bundle)

	if d.Capture.AgentID != "agent_test" {
		t.Errorf("Capture.AgentID: got %q", d.Capture.AgentID)
	}
	if !bytes.Equal(d.Capture.AgentDEK, bundle.AgentDEK) {
		t.Errorf("Capture.AgentDEK: got %v", d.Capture.AgentDEK)
	}
	if d.Capture.IndexName != "team-index" {
		t.Errorf("Capture.IndexName: got %q", d.Capture.IndexName)
	}
}

func TestApplyVaultBundle_PropagatesToRecall(t *testing.T) {
	d := newDepsForApply()
	bundle := &vault.Bundle{IndexName: "ix"}

	d.ApplyVaultBundle(bundle)

	if d.Recall.IndexName != "ix" {
		t.Errorf("Recall.IndexName: got %q, want ix", d.Recall.IndexName)
	}
}

func TestApplyVaultBundle_PropagatesToLifecycle(t *testing.T) {
	d := newDepsForApply()
	bundle := &vault.Bundle{
		IndexName: "ix",
		KeyID:     "key_z",
		AgentDEK:  bytes.Repeat([]byte{0x01}, 32),
		EncKey:    []byte("foo"),
	}

	d.ApplyVaultBundle(bundle)

	if d.Lifecycle.IndexName != "ix" {
		t.Errorf("Lifecycle.IndexName: got %q", d.Lifecycle.IndexName)
	}
	if d.Lifecycle.KeyID != "key_z" {
		t.Errorf("Lifecycle.KeyID: got %q", d.Lifecycle.KeyID)
	}
	if !bytes.Equal(d.Lifecycle.AgentDEK, bundle.AgentDEK) {
		t.Errorf("Lifecycle.AgentDEK mismatch")
	}
	if !d.Lifecycle.EncKeyLoaded {
		t.Error("Lifecycle.EncKeyLoaded: got false, want true (EncKey present)")
	}
}

func TestApplyVaultBundle_EncKeyLoadedFalseWhenEmpty(t *testing.T) {
	d := newDepsForApply()
	d.ApplyVaultBundle(&vault.Bundle{EncKey: nil})

	if d.Lifecycle.EncKeyLoaded {
		t.Error("EncKeyLoaded with nil EncKey: got true, want false")
	}
}

func TestApplyVaultBundle_NilBundleNoOp(t *testing.T) {
	d := newDepsForApply()
	d.Capture.AgentID = "preexisting"

	// Should not panic, should not modify state.
	d.ApplyVaultBundle(nil)

	if d.Capture.AgentID != "preexisting" {
		t.Errorf("nil bundle should be no-op, but Capture.AgentID changed to %q", d.Capture.AgentID)
	}
}

func TestApplyVaultBundle_NilServicesNoOp(t *testing.T) {
	// All service pointers nil → ApplyVaultBundle must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil services panicked: %v", r)
		}
	}()
	d := &mcp.Deps{} // no services
	d.ApplyVaultBundle(&vault.Bundle{AgentID: "x"})
}

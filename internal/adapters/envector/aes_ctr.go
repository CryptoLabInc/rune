package envector

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
)

// AES-256-CTR envelope for metadata.
// Spec: docs/v04/spec/components/rune-mcp.md §AES envelope.
// Python: mcp/adapter/envector_sdk.py:L227-234 _app_encrypt_metadata
//	+ pyenvector/utils/aes.py:L52-58.
//
// Format: {"a": agent_id, "c": base64(IV(16B) || CT)}
//   - "a" = agent_id (Vault bundle)
//   - "c" = base64(IV || AES-256-CTR(agent_dek, plaintext_utf8))
//   - No MAC (malleability present — Q1 AES-MAC Deferred)
//   - No AAD (meaningless in CTR)
//
// Capture path: Seal here.
// Recall path: service layer calls Vault.DecryptMetadata (Vault owns agent_dek
// for recall too — audit trail). This adapter does NOT call Open.

type envelope struct {
	A string `json:"a"` // agent_id
	C string `json:"c"` // base64(IV || ciphertext)
}

// Encrypts plaintext with AES-256-CTR using random 16-byte IV
// Returns: {"a": "<agent_id>", "c": "<base64(IV||CT)>"}
func Seal(dek []byte, agentID string, plaintext []byte) (string, error) {
	if len(dek) != 32 {
		return "", fmt.Errorf("envector: invalid DEK size %d (expected 32)", len(dek))
	}

	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", fmt.Errorf("envector: aes.NewCipher: %w", err)
	}

	// 16-byte random IV
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("envector: rand IV: %w", err)
	}

	// CTR encrypt
	ct := make([]byte, len(plaintext))
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ct, plaintext)

	// iv || ct -> base64
	combined := append(iv, ct...)
	b64 := base64.StdEncoding.EncodeToString(combined)

	env := envelope{A: agentID, C: b64}
	data, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("envector: json marshal envelope: %w", err)
	}
	return string(data), nil
}

// Open — reserved for potential local-decrypt path (currently Vault-delegated).
// Keep as interface for testing; production uses Vault.DecryptMetadata.
func Open(dek []byte, agentID string, envelopeStr string) ([]byte, error) {
	if len(dek) != 32 {
		return nil, fmt.Errorf("envector: invalid DEK size %d (expected 32)", len(dek))
	}

	var env envelope
	if err := json.Unmarshal([]byte(envelopeStr), &env); err != nil {
		return nil, fmt.Errorf("envector: invalid envelope JSON: %w", err)
	}

	if env.A != agentID {
		return nil, fmt.Errorf("envector: agent_id mismatch: got %q, want %q", env.A, agentID)
	}

	combined, err := base64.StdEncoding.DecodeString(env.C)
	if err != nil {
		return nil, fmt.Errorf("envector: base64 decode: %w", err)
	}

	if len(combined) < aes.BlockSize {
		return nil, fmt.Errorf("envector: ciphertext too short (len=%d)", len(combined))
	}

	iv := combined[:aes.BlockSize]
	ct := combined[aes.BlockSize:]

	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("envector: aes.NewCipher: %w", err)
	}

	plaintext := make([]byte, len(ct))
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(plaintext, ct)

	return plaintext, nil
}

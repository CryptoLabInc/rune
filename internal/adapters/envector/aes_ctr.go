package envector

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

// Seal — Python envector_sdk.py:L227-234.
// TODO: crypto/aes + crypto/cipher.NewCTR + crypto/rand (16B IV) + base64 + JSON marshal.
func Seal(dek []byte, agentID string, plaintext []byte) (string, error) {
	// TODO: bit-identical to _app_encrypt_metadata
	return "", nil
}

// Open — reserved for potential local-decrypt path (currently Vault-delegated).
// Keep as interface for testing; production uses Vault.DecryptMetadata.
// TODO: mirror Seal for tests.
func Open(dek []byte, agentID string, envelope string) ([]byte, error) {
	// TODO
	return nil, nil
}

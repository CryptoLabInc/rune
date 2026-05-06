package keymanager

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/envector/rune-go/internal/adapters/config"
)

type encKeyFile struct {
	Key string `json:"enc_key"`
}


func SaveKeys(keyID string, encKey []byte) error {
	runedir, err := config.RuneDir()
	if err != nil {
		return err
	}

	keyDir := filepath.Join(runedir, "keys", keyID)
	if err := os.MkdirAll(keyDir, config.DirPerm); err != nil {
		return fmt.Errorf("keymanager: mkdir %s: %w", keyDir, err)
	}

	if len(encKey) > 0 {
		encPath := filepath.Join(keyDir, "EncKey.json")
		b64 := base64.StdEncoding.EncodeToString(encKey)
		encData, _ := json.Marshal(encKeyFile{Key: b64})
		if err := os.WriteFile(encPath, encData, config.FilePerm); err != nil {
			return fmt.Errorf("keymanager: write EncKey.json: %w", err)
		}
	}

	return nil
}

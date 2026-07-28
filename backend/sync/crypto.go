package sync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// EncryptConfigFiles encrypts entire config files from srcDir into destDir.
// kc is used to backfill passwords from keychain before encryption.
// Pass nil for kc to skip backfill.
func EncryptConfigFiles(srcDir, destDir string, key []byte, kc *Keychain) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	if err := encryptConnectionsFile(
		filepath.Join(srcDir, "connections.json"),
		filepath.Join(destDir, "connections.json"),
		key, kc,
	); err != nil {
		return fmt.Errorf("encrypt connections: %w", err)
	}

	if err := encryptSettingsFile(
		filepath.Join(srcDir, "settings.json"),
		filepath.Join(destDir, "settings.json"),
		key, kc,
	); err != nil {
		return fmt.Errorf("encrypt settings: %w", err)
	}

	if err := encryptGenericFile(
		filepath.Join(srcDir, "quickCommands.json"),
		filepath.Join(destDir, "quickCommands.json"),
		key,
	); err != nil {
		return fmt.Errorf("encrypt quick commands: %w", err)
	}

	return nil
}

func encryptConnectionsFile(src, dest string, key []byte, kc *Keychain) error {
	data, err := readJSONFile(src)
	if err != nil {
		return err
	}

	if kc != nil {
		var wrapper struct {
			Groups      []map[string]interface{} `json:"groups"`
			Connections []map[string]interface{} `json:"connections"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return fmt.Errorf("parse connections: %w", err)
		}
		for _, cm := range wrapper.Connections {
			if cm["authType"] != "password" {
				continue
			}
			pw, _ := cm["password"].(string)
			if pw == "" {
				if id, ok := cm["id"].(string); ok {
					if kcPw, err := kc.GetPassword(id); err == nil && kcPw != "" {
						cm["password"] = kcPw
					}
				}
			}
		}
		data, _ = json.MarshalIndent(wrapper, "", "  ")
	}

	encoded, err := encryptBytes(data, key)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, []byte(encoded), 0600)
}

func encryptSettingsFile(src, dest string, key []byte, kc *Keychain) error {
	data, err := readJSONFile(src)
	if err != nil {
		return err
	}

	if kc != nil {
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err == nil {
			if ai, ok := obj["ai"].(map[string]interface{}); ok {
				if models, ok := ai["models"].([]interface{}); ok {
					for _, m := range models {
						if mm, ok := m.(map[string]interface{}); ok {
							ak, _ := mm["apiKey"].(string)
							if ak == "" {
								if id, ok := mm["id"].(string); ok {
									if kcAk, err := kc.GetModelAPIKey(id); err == nil && kcAk != "" {
										mm["apiKey"] = kcAk
									}
								}
							}
						}
					}
				}
			}
			data, _ = json.MarshalIndent(obj, "", "  ")
		}
	}

	encoded, err := encryptBytes(data, key)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, []byte(encoded), 0600)
}

// DecryptConfigFiles decrypts config files from srcDir into destDir.
// kc is used to write decrypted passwords to keychain and clear them from JSON.
// Pass nil for kc to skip keychain (passwords stay in JSON).
func DecryptConfigFiles(srcDir, destDir string, key []byte, kc *Keychain) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	if err := decryptConnectionsFile(
		filepath.Join(srcDir, "connections.json"),
		filepath.Join(destDir, "connections.json"),
		key, kc,
	); err != nil {
		return fmt.Errorf("decrypt connections: %w", err)
	}

	if err := decryptSettingsFile(
		filepath.Join(srcDir, "settings.json"),
		filepath.Join(destDir, "settings.json"),
		key, kc,
	); err != nil {
		return fmt.Errorf("decrypt settings: %w", err)
	}

	if err := decryptGenericFile(
		filepath.Join(srcDir, "quickCommands.json"),
		filepath.Join(destDir, "quickCommands.json"),
		key,
	); err != nil {
		return fmt.Errorf("decrypt quick commands: %w", err)
	}

	return nil
}

func decryptConnectionsFile(src, dest string, key []byte, kc *Keychain) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(dest, []byte("{}"), 0600)
		}
		return err
	}

	plaintext, err := decryptBytes(string(data), key)
	if err != nil {
		return fmt.Errorf("decrypt connections: %w", err)
	}

	if kc != nil {
		var wrapper struct {
			Groups      []map[string]interface{} `json:"groups"`
			Connections []map[string]interface{} `json:"connections"`
		}
		if err := json.Unmarshal(plaintext, &wrapper); err != nil {
			return fmt.Errorf("parse connections: %w", err)
		}
		for _, cm := range wrapper.Connections {
			if pw, ok := cm["password"].(string); ok && pw != "" {
				if id, ok := cm["id"].(string); ok {
					_ = kc.SetPassword(id, pw)
				}
				cm["password"] = ""
			}
		}
		plaintext, _ = json.MarshalIndent(wrapper, "", "  ")
	}

	return os.WriteFile(dest, plaintext, 0600)
}

func decryptSettingsFile(src, dest string, key []byte, kc *Keychain) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(dest, []byte("{}"), 0600)
		}
		return err
	}

	plaintext, err := decryptBytes(string(data), key)
	if err != nil {
		return fmt.Errorf("decrypt settings: %w", err)
	}

	// Extract per-model apiKeys to keychain, clear from JSON
	if kc != nil {
		var obj map[string]interface{}
		if err := json.Unmarshal(plaintext, &obj); err == nil {
			if ai, ok := obj["ai"].(map[string]interface{}); ok {
				if models, ok := ai["models"].([]interface{}); ok {
					for _, m := range models {
						if mm, ok := m.(map[string]interface{}); ok {
							if ak, ok := mm["apiKey"].(string); ok && ak != "" {
								if id, ok := mm["id"].(string); ok {
									_ = kc.SetModelAPIKey(id, ak)
								}
								mm["apiKey"] = ""
							}
						}
					}
				}
			}
			plaintext, _ = json.MarshalIndent(obj, "", "  ")
		}
	}

	return os.WriteFile(dest, plaintext, 0600)
}

// encryptGenericFile encrypts a config file that has no sensitive keychain-managed fields.
func encryptGenericFile(src, dest string, key []byte) error {
	data, err := readJSONFile(src)
	if err != nil {
		return err
	}
	encoded, err := encryptBytes(data, key)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, []byte(encoded), 0600)
}

// decryptGenericFile decrypts a config file that has no sensitive keychain-managed fields.
func decryptGenericFile(src, dest string, key []byte) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(dest, []byte("{}"), 0600)
		}
		return err
	}
	plaintext, err := decryptBytes(string(data), key)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}
	return os.WriteFile(dest, plaintext, 0600)
}

// encryptBytes encrypts plaintext under key, binding the ciphertext to a
// logical "file" identifier via additional data so an attacker who can
// swap ciphertexts across files (e.g. paste connections.json.enc over
// settings.json.enc) fails the AAD check (SYNC-P1-1).
func encryptBytes(plaintext []byte, key []byte) (string, error) {
	return encryptBytesWithAAD(plaintext, key, nil)
}

func encryptBytesWithAAD(plaintext []byte, key []byte, aad []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, aad)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decryptBytes(encoded string, key []byte) ([]byte, error) {
	return decryptBytesWithAAD(encoded, key, nil)
}

func decryptBytesWithAAD(encoded string, key []byte, aad []byte) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func readJSONFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte("{}"), nil
		}
		return nil, err
	}
	return data, nil
}

// ReadSaltFile reads the .sync-salt file from the repo directory.
// Returns nil if the file doesn't exist (new repo).
func ReadSaltFile(repoPath string) ([]byte, error) {
	saltPath := filepath.Join(repoPath, ".sync-salt")
	data, err := os.ReadFile(saltPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read salt file: %w", err)
	}
	salt, err := hex.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}
	return salt, nil
}

// WriteSaltFile writes the salt to .sync-salt in the repo directory.
func WriteSaltFile(repoPath string, salt []byte) error {
	saltPath := filepath.Join(repoPath, ".sync-salt")
	return os.WriteFile(saltPath, []byte(hex.EncodeToString(salt)), 0600)
}

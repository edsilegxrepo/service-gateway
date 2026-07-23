package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
)

var (
	ErrInvalidKey          = errors.New("invalid key: expected 64-char hex or 32-byte raw")
	ErrNoKeySource         = errors.New("no master key found")
	ErrInsecureLocation    = errors.New("insecure key location")
	ErrInsecurePermissions = errors.New("insecure file permissions")
	ErrCiphertextTooShort  = errors.New("ciphertext too short")
	ErrNotEncrypted        = errors.New("value is not encrypted")
)

const (
	EncryptedPrefix = "ENC["
	EncryptedSuffix = "]"
)

var (
	globalKey   []byte
	globalKeyMu sync.RWMutex
)

func InitMasterKey(keyPath, envVar string) error {
	key, err := ResolveKey(context.Background(), "", envVar, keyPath)
	if err != nil {
		return err
	}

	globalKeyMu.Lock()
	globalKey = key
	globalKeyMu.Unlock()

	return nil
}

func ResolveKey(ctx context.Context, raw, env, file string) ([]byte, error) {
	var keyStr string

	if raw != "" {
		keyStr = raw
	} else if env != "" {
		keyStr = os.Getenv(env)
	}

	if keyStr == "" && file != "" {
		if err := validateFilePermissions(file); err != nil {
			return nil, err
		}
		// #nosec G304 -- file path from admin config (master_key_path), validated above
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		keyStr = strings.TrimSpace(string(data))
	}

	if keyStr == "" {
		return nil, ErrNoKeySource
	}

	var key []byte
	var err error

	if len(keyStr) == 64 {
		key, err = hex.DecodeString(keyStr)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
		}
	} else if len(keyStr) == 32 {
		key = []byte(keyStr)
	} else {
		return nil, fmt.Errorf("%w: got %d chars", ErrInvalidKey, len(keyStr))
	}

	return key, nil
}

func Encrypt(plain string) (string, error) {
	globalKeyMu.RLock()
	key := globalKey
	globalKeyMu.RUnlock()

	if len(key) != 32 {
		return "", ErrInvalidKey
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plain), nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	return EncryptedPrefix + encoded + EncryptedSuffix, nil
}

func Decrypt(encrypted string) (string, error) {
	if !IsEncrypted(encrypted) {
		return encrypted, nil
	}

	encoded := strings.TrimPrefix(encrypted, EncryptedPrefix)
	encoded = strings.TrimSuffix(encoded, EncryptedSuffix)

	globalKeyMu.RLock()
	key := globalKey
	globalKeyMu.RUnlock()

	if len(key) != 32 {
		return "", ErrInvalidKey
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrCiphertextTooShort
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	return string(plaintext), nil
}

func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, EncryptedPrefix) && strings.HasSuffix(value, EncryptedSuffix)
}

func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	return hex.EncodeToString(key), nil
}

func validateFilePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		mode := info.Mode().Perm()
		if mode != 0o400 && mode != 0o600 {
			return fmt.Errorf("%w: %o (expected 0400 or 0600)", ErrInsecurePermissions, mode)
		}
	} else {
		if err := validateWindowsKeyPath(path); err != nil {
			return err
		}
	}
	return nil
}

func validateWindowsKeyPath(path string) error {
	absPath, err := getCanonicalPath(path)
	if err != nil {
		return fmt.Errorf("cannot resolve path: %w", err)
	}
	lowerPath := strings.ToLower(absPath)

	unsafePaths := []string{
		"\\users\\public\\",
		"\\programdata\\",
		"\\windows\\temp\\",
		"\\temp\\",
		"/temp/",
		"\\appdata\\local\\temp\\",
	}
	for _, unsafe := range unsafePaths {
		if strings.Contains(lowerPath, unsafe) {
			return fmt.Errorf("%w: %s", ErrInsecureLocation, path)
		}
	}

	userProfile := os.Getenv("USERPROFILE")
	if userProfile != "" {
		userProfile = strings.ToLower(userProfile)
		if !strings.HasPrefix(lowerPath, userProfile) {
			return fmt.Errorf("%w: key file should be in user profile directory", ErrInsecureLocation)
		}
	}

	return nil
}

func getCanonicalPath(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("symlinks not allowed for key files")
	}

	absPath, err := absPath(path)
	if err != nil {
		return "", err
	}
	return absPath, nil
}

func absPath(path string) (string, error) {
	if strings.HasPrefix(path, "\\\\?\\") {
		return path, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if len(path) >= 2 && path[1] == ':' {
		return path, nil
	}
	if strings.HasPrefix(path, "\\") || strings.HasPrefix(path, "/") {
		if len(wd) >= 2 && wd[1] == ':' {
			return wd[:2] + path, nil
		}
		return path, nil
	}
	return wd + "\\" + path, nil
}

func ZeroBuffer(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

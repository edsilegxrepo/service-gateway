package crypto

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if len(key) != 64 {
		t.Errorf("expected 64-char hex key, got %d chars", len(key))
	}

	key2, _ := GenerateKey()
	if key == key2 {
		t.Error("GenerateKey should produce unique keys")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key, _ := GenerateKey()
	keyBytes, _ := ResolveKey(nil, key, "", "")

	globalKeyMu.Lock()
	globalKey = keyBytes
	globalKeyMu.Unlock()

	tests := []struct {
		name  string
		plain string
	}{
		{"simple", "hello world"},
		{"empty", ""},
		{"unicode", "日本語テスト"},
		{"special chars", "pass!@#$%^&*()"},
		{"long", string(make([]byte, 10000))},
		{"json", `{"key": "value", "number": 123}`},
		{"url", "mongodb://user:pass@host:27017/db?ssl=true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := Encrypt(tt.plain)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			if !IsEncrypted(encrypted) {
				t.Error("IsEncrypted should return true for encrypted value")
			}

			if encrypted == tt.plain {
				t.Error("encrypted should differ from plaintext")
			}

			decrypted, err := Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			if decrypted != tt.plain {
				t.Errorf("decrypted = %q, want %q", decrypted, tt.plain)
			}
		})
	}
}

func TestDecryptPlaintext(t *testing.T) {
	plain := "not-encrypted-value"
	result, err := Decrypt(plain)
	if err != nil {
		t.Fatalf("Decrypt plaintext failed: %v", err)
	}
	if result != plain {
		t.Errorf("expected passthrough, got %q", result)
	}
}

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"ENC[abc123]", true},
		{"ENC[]", true},
		{"ENC[", false},
		{"]", false},
		{"plaintext", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsEncrypted(tt.value); got != tt.expected {
			t.Errorf("IsEncrypted(%q) = %v, want %v", tt.value, got, tt.expected)
		}
	}
}

func TestResolveKeyFromRaw(t *testing.T) {
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	key, err := ResolveKey(nil, hexKey, "", "")
	if err != nil {
		t.Fatalf("ResolveKey failed: %v", err)
	}

	if len(key) != 32 {
		t.Errorf("expected 32-byte key, got %d bytes", len(key))
	}
}

func TestResolveKeyFromEnv(t *testing.T) {
	hexKey := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	if err := os.Setenv("TEST_GATEWAY_KEY", hexKey); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_GATEWAY_KEY"); err != nil {
			t.Errorf("Unsetenv failed: %v", err)
		}
	}()

	key, err := ResolveKey(context.Background(), "", "TEST_GATEWAY_KEY", "")
	if err != nil {
		t.Fatalf("ResolveKey from env failed: %v", err)
	}

	if len(key) != 32 {
		t.Errorf("expected 32-byte key, got %d bytes", len(key))
	}
}

func TestResolveKeyFromFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission validation differs on Windows temp paths")
	}

	hexKey := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "test.key")

	if err := os.WriteFile(keyFile, []byte(hexKey), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	key, err := ResolveKey(nil, "", "", keyFile)
	if err != nil {
		t.Fatalf("ResolveKey from file failed: %v", err)
	}

	if len(key) != 32 {
		t.Errorf("expected 32-byte key, got %d bytes", len(key))
	}
}

func TestResolveKeyInvalidLength(t *testing.T) {
	_, err := ResolveKey(nil, "tooshort", "", "")
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestResolveKeyNoSource(t *testing.T) {
	_, err := ResolveKey(nil, "", "", "")
	if err != ErrNoKeySource {
		t.Errorf("expected ErrNoKeySource, got %v", err)
	}
}

func TestInsecureFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not applicable on Windows")
	}

	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "insecure.key")
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	if err := os.WriteFile(keyFile, []byte(hexKey), 0o644); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	_, err := ResolveKey(nil, "", "", keyFile)
	if err == nil {
		t.Error("expected error for insecure permissions")
	}
}

func TestZeroBuffer(t *testing.T) {
	buf := []byte{1, 2, 3, 4, 5}
	ZeroBuffer(buf)

	for i, b := range buf {
		if b != 0 {
			t.Errorf("buf[%d] = %d, want 0", i, b)
		}
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key, _ := GenerateKey()
	keyBytes, _ := ResolveKey(nil, key, "", "")

	globalKeyMu.Lock()
	globalKey = keyBytes
	globalKeyMu.Unlock()

	plain := "same plaintext"
	enc1, _ := Encrypt(plain)
	enc2, _ := Encrypt(plain)

	if enc1 == enc2 {
		t.Error("encrypting same plaintext should produce different ciphertexts (random nonce)")
	}
}

func TestEncryptInvalidKey(t *testing.T) {
	globalKeyMu.Lock()
	globalKey = []byte("short")
	globalKeyMu.Unlock()

	_, err := Encrypt("test")
	if err == nil {
		t.Error("expected error for invalid key length")
	}
}

func TestDecryptInvalidKey(t *testing.T) {
	globalKeyMu.Lock()
	globalKey = []byte("short")
	globalKeyMu.Unlock()

	_, err := Decrypt("ENC[invalid]")
	if err == nil {
		t.Error("expected error for invalid key")
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	key, _ := GenerateKey()
	keyBytes, _ := ResolveKey(nil, key, "", "")

	globalKeyMu.Lock()
	globalKey = keyBytes
	globalKeyMu.Unlock()

	_, err := Decrypt("ENC[not-valid-base64!!!]")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestDecryptTamperedData(t *testing.T) {
	key, _ := GenerateKey()
	keyBytes, _ := ResolveKey(nil, key, "", "")

	globalKeyMu.Lock()
	globalKey = keyBytes
	globalKeyMu.Unlock()

	encrypted, _ := Encrypt("secret")
	tampered := encrypted[:len(encrypted)-2] + "XX]"

	_, err := Decrypt(tampered)
	if err == nil {
		t.Error("expected error for tampered ciphertext")
	}
}

func TestInitMasterKey(t *testing.T) {
	if err := os.Setenv("TEST_INIT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_INIT_KEY"); err != nil {
			t.Errorf("Unsetenv failed: %v", err)
		}
	}()

	err := InitMasterKey("", "TEST_INIT_KEY")
	if err != nil {
		t.Fatalf("InitMasterKey failed: %v", err)
	}

	globalKeyMu.RLock()
	keyLen := len(globalKey)
	globalKeyMu.RUnlock()

	if keyLen != 32 {
		t.Errorf("expected 32-byte key, got %d", keyLen)
	}
}

func TestInitMasterKeyNoSource(t *testing.T) {
	err := InitMasterKey("", "")
	if err == nil {
		t.Error("expected error when no key source")
	}
}

func TestResolveKey32ByteRaw(t *testing.T) {
	raw32 := "12345678901234567890123456789012"

	key, err := ResolveKey(nil, raw32, "", "")
	if err != nil {
		t.Fatalf("ResolveKey 32-byte raw failed: %v", err)
	}

	if len(key) != 32 {
		t.Errorf("expected 32-byte key, got %d", len(key))
	}
}

func TestResolveKeyInvalidHex(t *testing.T) {
	invalidHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789ZZZZZZ"

	_, err := ResolveKey(nil, invalidHex, "", "")
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestDecryptShortCiphertext(t *testing.T) {
	key, _ := GenerateKey()
	keyBytes, _ := ResolveKey(context.Background(), key, "", "")

	globalKeyMu.Lock()
	globalKey = keyBytes
	globalKeyMu.Unlock()

	_, err := Decrypt("ENC[YWJj]")
	if err == nil {
		t.Error("expected error for short ciphertext")
	}
}

func TestResolveKeyPrecedence(t *testing.T) {
	rawKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	envKey := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	if err := os.Setenv("TEST_PRECEDENCE_KEY", envKey); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_PRECEDENCE_KEY"); err != nil {
			t.Errorf("Unsetenv failed: %v", err)
		}
	}()

	key, err := ResolveKey(context.Background(), rawKey, "TEST_PRECEDENCE_KEY", "")
	if err != nil {
		t.Fatalf("ResolveKey failed: %v", err)
	}

	expected, _ := hex.DecodeString(rawKey)
	if string(key) != string(expected) {
		t.Error("raw key should take precedence over env")
	}
}

func TestValidateFilePermissionsNonexistent(t *testing.T) {
	err := validateFilePermissions("/nonexistent/file")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, _ := GenerateKey()
	keyBytes, _ := ResolveKey(context.Background(), key, "", "")

	globalKeyMu.Lock()
	globalKey = keyBytes
	globalKeyMu.Unlock()

	testCases := []string{
		"simple text",
		"",
		"with\nnewlines\n",
		"unicode: 日本語 émoji 🎉",
		string(make([]byte, 1000)),
	}

	for _, tc := range testCases {
		enc, err := Encrypt(tc)
		if err != nil {
			t.Errorf("Encrypt(%q) failed: %v", tc[:min(len(tc), 20)], err)
			continue
		}

		dec, err := Decrypt(enc)
		if err != nil {
			t.Errorf("Decrypt failed: %v", err)
			continue
		}

		if dec != tc {
			t.Errorf("roundtrip failed for %q", tc[:min(len(tc), 20)])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestValidateFilePermissionsWindowsTemp(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "test.key")
	if err := os.WriteFile(keyFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := validateFilePermissions(keyFile)
	if err == nil {
		t.Error("temp path should be rejected on Windows")
	}
}

func TestValidateFilePermissionsWindowsInsecure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	err := validateFilePermissions("C:\\Users\\Public\\test.key")
	if err == nil {
		t.Error("expected error for Public path")
	}
}

func TestGenerateKeyUniqueness(t *testing.T) {
	keys := make(map[string]bool)
	for i := 0; i < 50; i++ {
		key, err := GenerateKey()
		if err != nil {
			t.Fatalf("GenerateKey failed: %v", err)
		}
		if keys[key] {
			t.Error("duplicate key generated")
		}
		keys[key] = true
	}
}

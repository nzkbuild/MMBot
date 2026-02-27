package secretbox

import (
	"encoding/base64"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	box, err := New(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("failed to create box: %v", err)
	}
	ciphertext, err := box.Encrypt("super-secret")
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	plaintext, err := box.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if plaintext != "super-secret" {
		t.Fatalf("unexpected plaintext: %s", plaintext)
	}
}

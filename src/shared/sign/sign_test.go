package sign

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type samplePayload struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	doc, err := SignJSON(kp.PrivateKey, samplePayload{Message: "parity report", Count: 42})
	if err != nil {
		t.Fatalf("SignJSON failed: %v", err)
	}

	if doc.PublicKey != kp.Fingerprint() {
		t.Errorf("expected public key %s, got %s", kp.Fingerprint(), doc.PublicKey)
	}

	if err := Verify(doc); err != nil {
		t.Errorf("expected verification to succeed, got: %v", err)
	}
}

func TestVerifyFailsOnTamperedPayload(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	doc, err := SignJSON(kp.PrivateKey, samplePayload{Message: "original", Count: 1})
	if err != nil {
		t.Fatalf("SignJSON failed: %v", err)
	}

	var tampered samplePayload
	if err := json.Unmarshal(doc.Payload, &tampered); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	tampered.Count = 999
	tamperedRaw, _ := json.Marshal(tampered)
	doc.Payload = tamperedRaw

	if err := Verify(doc); err == nil {
		t.Error("expected verification to fail on tampered payload, got nil error")
	}
}

func TestVerifyFailsOnTamperedSignature(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	doc, err := SignJSON(kp.PrivateKey, samplePayload{Message: "original", Count: 1})
	if err != nil {
		t.Fatalf("SignJSON failed: %v", err)
	}

	// Flip the signature's first character.
	sig := []byte(doc.Signature)
	if sig[0] == 'a' {
		sig[0] = 'b'
	} else {
		sig[0] = 'a'
	}
	doc.Signature = string(sig)

	if err := Verify(doc); err == nil {
		t.Error("expected verification to fail on tampered signature, got nil error")
	}
}

func TestVerifyAgainstTrustedKeyRejectsWrongKey(t *testing.T) {
	kp1, _ := GenerateKeyPair()
	kp2, _ := GenerateKeyPair()

	doc, err := SignJSON(kp1.PrivateKey, samplePayload{Message: "signed by kp1"})
	if err != nil {
		t.Fatalf("SignJSON failed: %v", err)
	}

	if err := VerifyAgainstTrustedKey(doc, kp2.PublicKey); err == nil {
		t.Error("expected verification against a different trusted key to fail")
	}
	if err := VerifyAgainstTrustedKey(doc, kp1.PublicKey); err != nil {
		t.Errorf("expected verification against the correct trusted key to succeed, got: %v", err)
	}
}

func TestSaveAndLoadKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	dir := t.TempDir()
	privPath := filepath.Join(dir, "key.priv")
	pubPath := filepath.Join(dir, "key.pub")

	if err := kp.SavePrivateKey(privPath); err != nil {
		t.Fatalf("SavePrivateKey failed: %v", err)
	}
	if err := kp.SavePublicKey(pubPath); err != nil {
		t.Fatalf("SavePublicKey failed: %v", err)
	}

	info, err := os.Stat(privPath)
	if err != nil {
		t.Fatalf("stat private key failed: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected private key file mode 0600, got %o", info.Mode().Perm())
	}

	loadedPriv, err := LoadPrivateKey(privPath)
	if err != nil {
		t.Fatalf("LoadPrivateKey failed: %v", err)
	}
	loadedPub, err := LoadPublicKey(pubPath)
	if err != nil {
		t.Fatalf("LoadPublicKey failed: %v", err)
	}

	doc, err := SignJSON(loadedPriv, samplePayload{Message: "roundtrip"})
	if err != nil {
		t.Fatalf("SignJSON with loaded key failed: %v", err)
	}
	if err := VerifyAgainstTrustedKey(doc, loadedPub); err != nil {
		t.Errorf("expected verification with loaded keys to succeed, got: %v", err)
	}
}

func TestLoadPrivateKeyRejectsWrongLength(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.priv")
	if err := os.WriteFile(badPath, []byte("deadbeef"), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	if _, err := LoadPrivateKey(badPath); err == nil {
		t.Error("expected LoadPrivateKey to reject a short key, got nil error")
	}
}

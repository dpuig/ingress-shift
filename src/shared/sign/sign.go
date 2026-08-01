// Package sign provides ed25519-based signing and verification for the
// artifacts customers present to a change advisory board or auditor: the
// harness's parity report and the orchestrator's remediation certificate.
//
// It deliberately uses only the Go standard library's crypto/ed25519 rather
// than an external KMS or signing service, so signing keeps working in an
// air-gapped environment (see PLAN.md's "on-premises, no phone-home" constraint).
package sign

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// KeyPair is an ed25519 key pair used to sign documents.
type KeyPair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// GenerateKeyPair creates a new random ed25519 key pair.
func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}
	return &KeyPair{PublicKey: pub, PrivateKey: priv}, nil
}

// Fingerprint returns the hex-encoded public key, used as a short identifier
// in signed documents and CLI output.
func (k *KeyPair) Fingerprint() string {
	return hex.EncodeToString(k.PublicKey)
}

// SavePrivateKey writes the hex-encoded private key to path with 0600 permissions.
func (k *KeyPair) SavePrivateKey(path string) error {
	return os.WriteFile(path, []byte(hex.EncodeToString(k.PrivateKey)), 0o600)
}

// SavePublicKey writes the hex-encoded public key to path with 0644 permissions.
func (k *KeyPair) SavePublicKey(path string) error {
	return os.WriteFile(path, []byte(hex.EncodeToString(k.PublicKey)), 0o644)
}

// LoadPrivateKey reads a hex-encoded ed25519 private key from path.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key %s: %w", path, err)
	}
	decoded, err := decodeHexKey(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid private key %s: %w", path, err)
	}
	if len(decoded) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key %s: expected %d bytes, got %d", path, ed25519.PrivateKeySize, len(decoded))
	}
	return ed25519.PrivateKey(decoded), nil
}

// LoadPublicKey reads a hex-encoded ed25519 public key from path.
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key %s: %w", path, err)
	}
	decoded, err := decodeHexKey(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid public key %s: %w", path, err)
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key %s: expected %d bytes, got %d", path, ed25519.PublicKeySize, len(decoded))
	}
	return ed25519.PublicKey(decoded), nil
}

func decodeHexKey(raw []byte) ([]byte, error) {
	trimmed := trimSpace(raw)
	decoded := make([]byte, hex.DecodedLen(len(trimmed)))
	n, err := hex.Decode(decoded, trimmed)
	if err != nil {
		return nil, err
	}
	return decoded[:n], nil
}

func trimSpace(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && isSpace(b[start]) {
		start++
	}
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\t' || c == '\r'
}

// Document is a JSON payload plus a detached ed25519 signature and the
// public key fingerprint needed to verify it, without requiring the
// verifier to already have the key on hand.
//
// Payload is deliberately []byte, not json.RawMessage: encoding/json
// base64-encodes a plain []byte field as an opaque string, whereas
// json.RawMessage is spliced in as structural JSON. That distinction
// matters because Go's indent-and-reformat pass (json.Encoder with
// SetIndent, which the CLI commands use to write human-readable files)
// rewrites whitespace through any nested structural JSON, silently
// changing the exact bytes that were signed. A base64 string's content is
// opaque to that reformatting, so the signed bytes survive being written
// to disk, pretty-printed, or re-encoded, no matter how the caller
// chooses to marshal the envelope around it.
type Document struct {
	Payload   []byte `json:"payload"`
	Signature string `json:"signature"`
	PublicKey string `json:"public_key"`
}

// SignJSON marshals payload to canonical JSON and produces a signed Document.
func SignJSON(priv ed25519.PrivateKey, payload any) (*Document, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("private key does not have an ed25519 public key")
	}

	sig := ed25519.Sign(priv, raw)

	return &Document{
		Payload:   raw,
		Signature: hex.EncodeToString(sig),
		PublicKey: hex.EncodeToString(pub),
	}, nil
}

// Verify checks a Document's signature against its embedded public key.
// It returns an error if the signature is invalid, missing, or has been tampered with.
func Verify(doc *Document) error {
	pubBytes, err := hex.DecodeString(doc.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid public key encoding: %w", err)
	}
	if len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key length: expected %d bytes, got %d", ed25519.PublicKeySize, len(pubBytes))
	}

	sigBytes, err := hex.DecodeString(doc.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}

	if !ed25519.Verify(ed25519.PublicKey(pubBytes), doc.Payload, sigBytes) {
		return fmt.Errorf("signature verification failed: document may have been tampered with")
	}

	return nil
}

// VerifyAgainstTrustedKey checks a Document's signature was produced by a
// specific trusted public key, not merely a self-consistent one embedded in
// the document. Use this when verifying a report received from someone
// else, where you must not trust the public key they shipped alongside it.
func VerifyAgainstTrustedKey(doc *Document, trustedPublicKey ed25519.PublicKey) error {
	if hex.EncodeToString(trustedPublicKey) != doc.PublicKey {
		return fmt.Errorf("document was signed by a different key than the trusted one provided")
	}
	return Verify(doc)
}

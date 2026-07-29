package signing

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Signer struct {
	privateKey *rsa.PrivateKey
	certPath   string
}

const KeyID = "grom-default"

func LoadOrCreate(keyPath, certPath string) (*Signer, error) {
	if keyPEM, err := os.ReadFile(keyPath); err == nil {
		block, _ := pem.Decode(keyPEM)
		if block == nil {
			return nil, fmt.Errorf("decode signing key")
		}
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse signing key: %w", err)
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("signing key is not RSA")
		}
		if _, err := os.Stat(certPath); err != nil {
			return nil, fmt.Errorf("signing certificate missing: %w", err)
		}
		return &Signer{privateKey: rsaKey, certPath: certPath}, nil
	}

	if err := os.MkdirAll(filepath.Dir(keyPath), 0o750); err != nil {
		return nil, fmt.Errorf("create signing key directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(certPath), 0o750); err != nil {
		return nil, fmt.Errorf("create signing certificate directory: %w", err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, fmt.Errorf("generate signing key: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal signing key: %w", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return nil, fmt.Errorf("write signing key: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "Grom Registry Token Signer"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create signing certificate: %w", err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0o644); err != nil {
		return nil, fmt.Errorf("write signing certificate: %w", err)
	}
	return &Signer{privateKey: key, certPath: certPath}, nil
}

func (s *Signer) Sign(claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = KeyID
	return token.SignedString(s.privateKey)
}

func (s *Signer) Verify(raw string, claims jwt.Claims, issuer, audience string) error {
	_, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return &s.privateKey.PublicKey, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
	)
	return err
}

func (s *Signer) CertificatePath() string {
	return s.certPath
}

func (s *Signer) WriteJWKS(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create JWKS directory: %w", err)
	}
	exponent := big.NewInt(int64(s.privateKey.PublicKey.E)).Bytes()
	document := struct {
		Keys []map[string]string `json:"keys"`
	}{
		Keys: []map[string]string{{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": KeyID,
			"n":   base64.RawURLEncoding.EncodeToString(s.privateKey.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(exponent),
		}},
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JWKS: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write JWKS: %w", err)
	}
	return nil
}

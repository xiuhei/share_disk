package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// tokenIssuer is the value required in the "iss" claim.
	tokenIssuer = "share-disk"
	// tokenAudience is the value required in the "aud" claim.
	tokenAudience = "share-disk"
)

// TokenClaims represents the access-token JWT claims.
type TokenClaims struct {
	jwt.RegisteredClaims
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id"`
	SessionID string `json:"session_id"`
	Scope     string `json:"scope,omitempty"`
	FileID    string `json:"file_id,omitempty"`
}

// TokenManager signs (server) or verifies (agent) access tokens using Ed25519.
// The server holds the private key and never shares it; Agents and other
// verifiers hold only the public key, so a compromised Agent cannot mint tokens
// that the backend or other Agents accept.
type TokenManager struct {
	signKey   ed25519.PrivateKey // nil when verify-only
	verifyKey ed25519.PublicKey
	keyID     string
	accessTTL time.Duration
}

// GenerateSigningKey creates a fresh Ed25519 key pair and returns both PEM
// forms. The private key must only ever be installed on the control server; the
// public key is distributed to Agents (directly or via JWKS).
func GenerateSigningKey() (privatePEM, publicPEM string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate Ed25519 key: %w", err)
	}
	return MarshalPrivateKeyPEM(priv), MarshalPublicKeyPEM(pub), nil
}

// MarshalPrivateKeyPEM encodes an Ed25519 private key as a PKCS#8 PEM block.
func MarshalPrivateKeyPEM(key ed25519.PrivateKey) string {
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// MarshalPublicKeyPEM encodes an Ed25519 public key as a PKIX PEM block.
func MarshalPublicKeyPEM(key ed25519.PublicKey) string {
	der, _ := x509.MarshalPKIXPublicKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// ParsePrivateKeyPEM decodes a PKCS#8 private key PEM into an Ed25519 key.
func ParsePrivateKeyPEM(pemData string) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("failed to decode private key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is %T, not Ed25519", parsed)
	}
	return key, nil
}

// ParsePublicKeyPEM decodes a PKIX public key PEM into an Ed25519 key.
func ParsePublicKeyPEM(pemData string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("failed to decode public key PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is %T, not Ed25519", parsed)
	}
	return key, nil
}

// NewTokenManager creates a signing TokenManager from an Ed25519 private key
// PEM. It is used by the control server.
func NewTokenManager(privateKeyPEM string, accessTTL time.Duration) (*TokenManager, error) {
	signKey, err := ParsePrivateKeyPEM(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	pub, ok := signKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("unexpected public key type %T", signKey.Public())
	}
	return &TokenManager{
		signKey:   signKey,
		verifyKey: pub,
		keyID:     keyID(pub),
		accessTTL: accessTTL,
	}, nil
}

// NewTokenVerifier creates a verify-only TokenManager from an Ed25519 public
// key PEM. It is used by the Agent, which must not hold signing material.
func NewTokenVerifier(publicKeyPEM string) (*TokenManager, error) {
	verifyKey, err := ParsePublicKeyPEM(publicKeyPEM)
	if err != nil {
		return nil, err
	}
	return &TokenManager{
		verifyKey: verifyKey,
		keyID:     keyID(verifyKey),
	}, nil
}

// keyID returns a stable identifier derived from the public key so token
// headers and the JWKS endpoint agree on which key to use.
func keyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])[:16]
}

// KeyID returns the key identifier embedded in signed tokens and the JWKS.
func (m *TokenManager) KeyID() string { return m.keyID }

// PublicKey returns the Ed25519 public key used for verification.
func (m *TokenManager) PublicKey() ed25519.PublicKey { return m.verifyKey }

// GenerateAccessToken signs a single access token for the given identity.
func (m *TokenManager) GenerateAccessToken(userID, deviceID, sessionID string) (string, error) {
	return m.generateToken(userID, deviceID, sessionID, "", "", m.accessTTL)
}

// GenerateShareAccessToken creates an Agent-verifiable token that can only be
// used to download one local file. The LAN middleware enforces this scope.
func (m *TokenManager) GenerateShareAccessToken(userID, localFileID, shareID string, ttl time.Duration) (string, error) {
	if localFileID == "" || shareID == "" || ttl <= 0 {
		return "", errors.New("share token requires file, share and positive ttl")
	}
	if ttl > m.accessTTL {
		ttl = m.accessTTL
	}
	return m.generateToken(userID, shareID, shareID, "share_download", localFileID, ttl)
}

func (m *TokenManager) generateToken(userID, deviceID, sessionID, scope, fileID string, ttl time.Duration) (string, error) {
	if m.signKey == nil {
		return "", errors.New("token manager has no signing key (verify-only)")
	}
	now := time.Now()
	claims := &TokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{tokenAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        generateTokenID(),
		},
		UserID:    userID,
		DeviceID:  deviceID,
		SessionID: sessionID,
		Scope:     scope,
		FileID:    fileID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = m.keyID
	return token.SignedString(m.signKey)
}

// AccessTokenTTL returns the access token time-to-live in seconds.
func (m *TokenManager) AccessTokenTTL() int64 {
	return int64(m.accessTTL.Seconds())
}

// ValidateAccessToken parses and verifies an access token, enforcing the
// signing algorithm, signature, issuer, audience, expiry and the presence of
// identity claims. It does not check server-side session/device revocation; the
// Service does that in Authenticate.
func (m *TokenManager) ValidateAccessToken(tokenString string) (*TokenClaims, error) {
	claims := &TokenClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return m.verifyKey, nil
	},
		jwt.WithIssuer(tokenIssuer),
		jwt.WithAudience(tokenAudience),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	// An attacker could otherwise strip the custom identity claims and still
	// have a structurally valid token.
	if claims.UserID == "" || claims.DeviceID == "" || claims.SessionID == "" {
		return nil, errors.New("token missing required identity claims")
	}
	if claims.Subject != claims.UserID {
		return nil, errors.New("token subject does not match user_id claim")
	}

	return claims, nil
}

// JWKS returns the JSON Web Key Set for the manager's public key, keyed by kid.
func (m *TokenManager) JWKS() map[string]interface{} {
	return map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "OKP",
				"crv": "Ed25519",
				"x":   base64.RawURLEncoding.EncodeToString(m.verifyKey),
				"kid": m.keyID,
				"alg": jwt.SigningMethodEdDSA.Alg(),
				"use": "sig",
			},
		},
	}
}

func generateTokenID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GenerateRefreshToken generates a random opaque refresh token string. Refresh
// tokens are stored server-side only as a salted hash and never signed, so they
// do not require asymmetric key material.
func GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

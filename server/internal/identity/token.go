package identity

import "github.com/share-disk/share-disk/internal/auth"

// Token types and constructors are aliases to the shared Ed25519 token
// implementation. The server owns token issuance while Ubuntu Agents import
// internal/auth directly and receive only the public verification key.
type TokenClaims = auth.TokenClaims
type TokenManager = auth.TokenManager

var NewTokenManager = auth.NewTokenManager
var GenerateRefreshToken = auth.GenerateRefreshToken

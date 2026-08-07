package auth

import (
	"context"
)

// Define a custom unexported type for the context key to prevent collisions.
type contextKey string

const claimsKey contextKey = "user_claims"

// ContextWithClaims returns a new context with the provided claims attached.
func ContextWithClaims(ctx context.Context, claims *CustomClaims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// ClaimsFromContext retrieves the claims from the context, if they exist.
func ClaimsFromContext(ctx context.Context) (*CustomClaims, bool) {
	claims, ok := ctx.Value(claimsKey).(*CustomClaims)
	return claims, ok
}

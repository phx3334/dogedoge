package request

import (
	jwt "github.com/golang-jwt/jwt/v4"
	"fake_tiktok/internal/domain/database"
)



type JwtCustomClaims struct {
	BaseClaims
	jwt.RegisteredClaims
}

type JwtCustomRefreshClaims struct {
	UserID string 
	jwt.RegisteredClaims
}

type BaseClaims struct {
	UserID string
	Role database.Role
}
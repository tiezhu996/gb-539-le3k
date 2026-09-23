package service

import (
	"context"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"timber-kiln-drying-optimizer/backend/internal/model"
	"timber-kiln-drying-optimizer/backend/internal/repository"
	"time"
)

type AuthService struct {
	Users  repository.UserRepository
	Secret string
}

func (s AuthService) Login(ctx context.Context, email, password string) (string, model.User, error) {
	user, err := s.Users.FindByEmail(ctx, email)
	if err != nil || !model.CheckPassword(user.PasswordHash, password) {
		return "", user, fmt.Errorf("invalid credentials: %w", ErrForbidden)
	}
	claims := jwt.MapClaims{"sub": user.ID, "email": user.Email, "role": user.Role, "name": user.Name, "exp": time.Now().Add(8 * time.Hour).Unix()}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.Secret))
	return token, user, err
}
func (s AuthService) Parse(token string) (jwt.MapClaims, error) {
	parsed, err := jwt.Parse(token, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(s.Secret), nil
	})
	if err != nil || !parsed.Valid {
		return nil, fmt.Errorf("parse token: %w", ErrForbidden)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrForbidden
	}
	return claims, nil
}

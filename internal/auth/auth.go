package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func HashPassword(password string) (string, error) {
	hashedPwd, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", err
	}

	return hashedPwd, nil
}

func CheckHashedPassword(password, hash string) (bool, error) {
	isMatch, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, err
	}

	return isMatch, nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	uuidStr := userID.String()
	timeNow := jwt.NumericDate{
		Time: time.Now(),
	}
	timeExpires := jwt.NumericDate{
		Time: time.Now().Add(expiresIn),
	}

	claims := jwt.RegisteredClaims{
		Issuer:    "chirpy-access",
		IssuedAt:  &timeNow,
		ExpiresAt: &timeExpires,
		Subject:   uuidStr,
	}

	newJwt := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := newJwt.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", err
	}

	return token, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claims := jwt.RegisteredClaims{}

	validateToken := func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			errMsg := errors.New("invalid signing method")
			return nil, errMsg
		}
		return []byte(tokenSecret), nil
	}

	token, err := jwt.ParseWithClaims(tokenString, &claims, validateToken)
	if err != nil {
		return uuid.UUID{}, err
	}

	if !token.Valid {
		errMsg := errors.New("invalid token")
		return uuid.UUID{}, errMsg
	}

	userIDstr, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.UUID{}, err
	}

	usrID, err := uuid.Parse(userIDstr)
	if err != nil {
		return uuid.UUID{}, err
	}

	return usrID, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	auth := headers.Get("Authorization")
	if auth == "" {
		errMsg := errors.New("authorization header not found")
		return "", errMsg
	}

	bearer := strings.TrimPrefix(auth, "Bearer ")

	return bearer, nil
}

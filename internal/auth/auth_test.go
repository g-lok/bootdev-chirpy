package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHashPassword(t *testing.T) {
	password := "apassword"
	hashedPass, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}

	if hashedPass == password {
		t.Error("hashedpassword is same as plaintext password")
	}
}

func TestHashedPasswordCheck(t *testing.T) {
	password := "password"
	hashedPass, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}

	isSame, err := CheckHashedPassword(password, hashedPass)
	if err != nil {
		t.Fatal(err)
	}

	if !isSame {
		t.Error("correct password failed to validate against hash")
	}

	hashedPassWrong, err := HashPassword("notthepassword")
	if err != nil {
		t.Fatal(err)
	}

	isSame2, err := CheckHashedPassword(password, hashedPassWrong)
	if err != nil {
		t.Fatal(err)
	}

	if isSame2 {
		t.Error("incorrect password succeeded hash check")
	}
}

func TestJwt(t *testing.T) {
	// whatever
	userID := uuid.New()
	tokenSecret := "mysecret"
	wrongTokenSecret := "notmysecret"

	reasonableExpire := time.Hour
	unreasonableExpire := -time.Second

	validJwt, err := MakeJWT(userID, tokenSecret, reasonableExpire)
	if err != nil {
		t.Fatal(err)
	}

	expiredJwt, err := MakeJWT(userID, tokenSecret, unreasonableExpire)
	if err != nil {
		t.Fatal(err)
	}

	invalidSecretJwt, err := MakeJWT(userID, wrongTokenSecret, reasonableExpire)
	if err != nil {
		t.Fatal(err)
	}

	validUUID, err := ValidateJWT(validJwt, tokenSecret)
	if err != nil {
		t.Fatal(err)
	}
	if validUUID != userID {
		t.Error("validJwt UUID did not match original UUID")
	}

	_, err = ValidateJWT(expiredJwt, tokenSecret)
	if err == nil {
		t.Error("expired token didn't raise error")
	}

	_, err = ValidateJWT(invalidSecretJwt, tokenSecret)
	if err == nil {
		t.Error("jwt with incorrect secret didn't raise error")
	}
}

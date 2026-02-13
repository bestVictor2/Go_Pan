package utils

import (
	"golang.org/x/crypto/bcrypt"
	_ "golang.org/x/crypto/bcrypt"
	"log"
)

// GetPwd hashes a password.
func GetPwd(pwd string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal("generate password error:", err)
	}
	return string(hash)
}

// CheckPwd verifies a password hash.
func CheckPwd(pwd string, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(password), []byte(pwd))
	if err != nil {
		return false
	} else {
		return true
	}
}


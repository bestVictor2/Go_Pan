package utils

import (
	"math/rand"
	"time"
)

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// GenExtractCode generates a share extract code.
func GenExtractCode() string {
	chars := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 4)
	for i := range code {
		code[i] = chars[rng.Intn(len(chars))]
	}
	return string(code)
}




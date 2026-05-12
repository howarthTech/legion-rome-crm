// hash-password — tiny utility for generating a bcrypt hash to drop into
// ADMIN_PASSWORD_HASH. Usage: go run ./cmd/hash-password 'my-password'
package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: hash-password <password>")
		os.Exit(1)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(os.Args[1]), 10)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash error:", err)
		os.Exit(1)
	}
	fmt.Println(string(h))
}

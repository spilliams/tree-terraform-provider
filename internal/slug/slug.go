// Package slug provides helper functions that generate random suffix "slugs".
package slug

import (
	"fmt"
	"math/rand"
	"strings"
)

const letters = "abcdefghijklmnopqrstuvwxyz"
const Separator = "_"

// not terribly fast, but only used when generating new IDs.
// also not cryptographically secure, but we don't need that.
func randSeq(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Int63()%int64(len(letters))]
	}
	return string(b)
}

func Generate(prefix string) string {
	return fmt.Sprintf("%s%s%s", prefix, Separator, randSeq(10))
}

func Type(id string) string {
	return strings.Split(id, Separator)[0]
}

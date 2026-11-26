package client

import (
	"fmt"
	"strings"
)

// Quote returns s as a PocketBase filter string literal.
func Quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// Eq builds a `field = "value"` filter expression with the value escaped.
func Eq(field, value string) string {
	return fmt.Sprintf("%s = %s", field, Quote(value))
}

// AnyEq builds a `field ?= "value"` filter expression, matching multi-value relations.
func AnyEq(field, value string) string {
	return fmt.Sprintf("%s ?= %s", field, Quote(value))
}

// And joins non-empty filter expressions with &&.
func And(exprs ...string) string {
	parts := make([]string, 0, len(exprs))
	for _, e := range exprs {
		if e != "" {
			parts = append(parts, "("+e+")")
		}
	}
	return strings.Join(parts, " && ")
}

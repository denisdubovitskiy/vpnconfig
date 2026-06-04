package integration_test

import "encoding/base64"

// base64Std обёртка над base64.StdEncoding.EncodeToString для
// изоляции импорта encoding/base64.
func base64Std(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

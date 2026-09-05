package cmd

import (
	"os"
	"strconv"
)

// envStr returns the value of the environment variable key, or def if it is
// unset. (An explicitly-set-but-empty value is honored.)
func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// envInt64 parses key as a base-10 int64, falling back to def when unset or
// malformed.
func envInt64(key string, def int64) int64 {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

// envInt parses key as an int, falling back to def when unset or malformed.
func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// envBool parses key as a boolean (1/t/true/0/f/false …), falling back to def
// when unset or malformed.
func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

package cmd

import "testing"

func TestEnvHelpers(t *testing.T) {
	t.Run("str", func(t *testing.T) {
		t.Setenv("HB_TEST_STR", "value")
		if got := envStr("HB_TEST_STR", "def"); got != "value" {
			t.Errorf("envStr set = %q, want %q", got, "value")
		}
		if got := envStr("HB_TEST_MISSING", "def"); got != "def" {
			t.Errorf("envStr unset = %q, want %q", got, "def")
		}
	})

	t.Run("int64", func(t *testing.T) {
		t.Setenv("HB_TEST_I64", "1048576")
		if got := envInt64("HB_TEST_I64", 7); got != 1048576 {
			t.Errorf("envInt64 set = %d, want 1048576", got)
		}
		t.Setenv("HB_TEST_I64", "not-a-number")
		if got := envInt64("HB_TEST_I64", 7); got != 7 {
			t.Errorf("envInt64 malformed = %d, want fallback 7", got)
		}
	})

	t.Run("bool", func(t *testing.T) {
		t.Setenv("HB_TEST_BOOL", "true")
		if !envBool("HB_TEST_BOOL", false) {
			t.Error("envBool=true should be true")
		}
		t.Setenv("HB_TEST_BOOL", "garbage")
		if envBool("HB_TEST_BOOL", false) {
			t.Error("envBool malformed should fall back to default (false)")
		}
	})
}

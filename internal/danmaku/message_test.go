package danmaku

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateMessageUsesWeightedUTF16BudgetWithoutChangingText(t *testing.T) {
	for _, tc := range []struct {
		name, accepted, overlong string
	}{
		{"ASCII", strings.Repeat("a", 20), strings.Repeat("a", 21)},
		{"Chinese", strings.Repeat("弹", 10), strings.Repeat("弹", 11)},
		{"emoji", strings.Repeat("🐱", 5), strings.Repeat("🐱", 6)},
		{"mixed", strings.Repeat("a", 14) + "弹🐱", strings.Repeat("a", 15) + "弹🐱"},
		{"combining", strings.Repeat("e\u0301", 6) + "ab", strings.Repeat("e\u0301", 7)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, ValidateMessage(tc.accepted))
			require.ErrorIs(t, ValidateMessage(tc.overlong), ErrTooLong)
		})
	}
	require.NoError(t, ValidateMessage("  hello  "))
	require.ErrorIs(t, ValidateMessage(" "+strings.Repeat("a", MaxMessageUnits)), ErrTooLong,
		"spaces that would be sent count toward the limit")
}

func TestValidateMessageRejectsInvalidText(t *testing.T) {
	for name, message := range map[string]string{
		"empty":         "",
		"whitespace":    " \t\n",
		"unicode space": "\u00a0\u3000",
		"newline":       "two\nlines",
		"null":          "text\x00",
		"invalid UTF8":  string([]byte{0xff}),
		"long control":  strings.Repeat("a", MaxMessageUnits+1) + "\x00",
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, ValidateMessage(message), ErrInvalidInput)
		})
	}
}

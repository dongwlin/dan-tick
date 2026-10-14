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
		{"ASCII", strings.Repeat("a", 80), strings.Repeat("a", 81)},
		{"Chinese", strings.Repeat("弹", 40), strings.Repeat("弹", 41)},
		{"fullwidth punctuation", strings.Repeat("。", 40), strings.Repeat("。", 41)},
		{"emoji", strings.Repeat("🐱", 20), strings.Repeat("🐱", 21)},
		{"mixed", strings.Repeat("a", 74) + "弹🐱", strings.Repeat("a", 75) + "弹🐱"},
		{"combining", strings.Repeat("e\u0301", 26) + "ab", strings.Repeat("e\u0301", 27)},
		{"skin tone", strings.Repeat("👍🏽", 10), strings.Repeat("👍🏽", 11)},
		{"variation selector", strings.Repeat("❤️", 20), strings.Repeat("❤️", 21)},
		{"ZWJ", strings.Repeat("👨‍👩‍👧‍👦", 3) + strings.Repeat("a", 14), strings.Repeat("👨‍👩‍👧‍👦", 3) + strings.Repeat("a", 15)},
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

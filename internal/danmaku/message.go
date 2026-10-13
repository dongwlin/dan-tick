package danmaku

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxMessageUnits is this application's conservative offline message budget.
// Bilibili may allow a different length depending on the account.
const MaxMessageUnits = 20

// ValidateMessage checks the exact text that would be sent without truncating
// or otherwise changing it. Printable ASCII costs one unit; every other UTF-16
// code unit costs two, so supplementary-plane characters cost four.
func ValidateMessage(message string) error {
	if !utf8.ValidString(message) || strings.TrimSpace(message) == "" {
		return ErrInvalidInput
	}
	units := 0
	for _, r := range message {
		if unicode.IsControl(r) {
			return ErrInvalidInput
		}
		switch {
		case r >= 0x20 && r <= 0x7e:
			units++
		case r <= 0xffff:
			units += 2
		default:
			units += 4
		}
	}
	if units > MaxMessageUnits {
		return ErrTooLong
	}
	return nil
}

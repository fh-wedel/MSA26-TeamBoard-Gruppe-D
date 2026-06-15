// Package ordering provides lexicographic fractional ranking for drag-and-drop ordering.
// Positions are strings over the alphabet "0-9A-Za-z" that sort correctly lexicographically.
// Between any two positions a new one can always be generated in O(1) without moving others.
package ordering

import (
	"errors"
	"strings"
)

const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var ErrNoSpaceLeft = errors.New("no space left between positions")

// Initial returns the position for the first element.
func Initial() string { return "U" }

// Between returns a position lexicographically between prev and next.
// Either may be empty: empty prev means "before everything", empty next means "after everything".
func Between(prev, next string) (string, error) {
	if prev == "" && next == "" {
		return Initial(), nil
	}
	if next == "" {
		return append_(prev), nil
	}
	if prev == "" {
		return prepend_(next)
	}
	if prev >= next {
		return "", ErrNoSpaceLeft
	}
	return midpoint(prev, next)
}

// append_ returns a position after prev (appends the middle character of the alphabet).
func append_(prev string) string {
	return prev + string(alphabet[len(alphabet)/2])
}

// prepend_ returns a position before next.
func prepend_(next string) (string, error) {
	// Try to decrement the last char.
	chars := []byte(next)
	for i := len(chars) - 1; i >= 0; i-- {
		idx := strings.IndexByte(alphabet, chars[i])
		if idx > 0 {
			chars[i] = alphabet[idx-1]
			return string(chars[:i+1]), nil
		}
		// idx == 0, need to borrow
	}
	// All chars are at minimum — prepend a '0' at beginning.
	return "0" + next, nil
}

// midpoint returns a lexicographic midpoint between prev and next.
// It scans left-to-right: equal chars are passed through, a gap of ≥2 is
// halved, and a gap of 1 is resolved by appending the rest of prev plus the
// middle-of-alphabet character (which is always strictly between prev and next
// when the leading characters differ by exactly one).
func midpoint(prev, next string) (string, error) {
	result := []byte{}
	for i := 0; ; i++ {
		// pc: numeric index of prev's i-th character (0 when prev is exhausted).
		pc := 0
		if i < len(prev) {
			pc = strings.IndexByte(alphabet, prev[i])
		}

		// nc: numeric index of next's i-th character (len(alphabet) as sentinel when
		// next is exhausted, guaranteeing a gap of at least len(alphabet)-pc).
		nc := len(alphabet)
		if i < len(next) {
			nc = strings.IndexByte(alphabet, next[i])
		}

		diff := nc - pc
		switch {
		case diff > 1:
			// Enough room to halve.
			result = append(result, alphabet[(pc+nc)/2])
			return trimRight(string(result)), nil

		case diff == 1:
			// Adjacent characters: include prev's char, then append the rest of prev
			// followed by the middle-alphabet character.  The result is guaranteed
			// to be strictly less than next because its leading character equals
			// prev[i] which is strictly less than next[i].
			result = append(result, alphabet[pc])
			if i+1 < len(prev) {
				result = append(result, prev[i+1:]...)
			}
			result = append(result, alphabet[len(alphabet)/2])
			return trimRight(string(result)), nil

		case diff == 0:
			// Same character: include and advance.
			result = append(result, alphabet[pc])

		default:
			// pc > nc — prev >= next, which the caller already checked.
			return "", ErrNoSpaceLeft
		}
	}
}

// trimRight removes trailing minimum characters (like removing trailing zeros).
func trimRight(s string) string {
	i := len(s)
	for i > 1 && s[i-1] == alphabet[0] {
		i--
	}
	return s[:i]
}

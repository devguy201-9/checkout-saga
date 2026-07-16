package entity

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Money is an exact monetary amount stored in minor units (cents).
//
// Money is never a float: binary floating point cannot represent values such as
// 0.10 exactly, so sums drift by cents. int64 minor units are exact, cheap to
// add, and map cleanly onto the DECIMAL(10,2) columns in order_db
// (see docs/design/order-db.md).
type Money int64

// _minorUnits is the number of minor units in one major unit (100 cents = 1).
const _minorUnits = 100

// _maxMajorDigits bounds the integer part so units*100 cannot overflow int64.
const _maxMajorDigits = 15

// ParseMoney parses decimal text ("10", "10.5", "10.50") into Money without
// ever touching a float. At most 2 decimal places are accepted, matching
// DECIMAL(10,2).
func ParseMoney(s string) (Money, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return 0, fmt.Errorf("%w: empty string", ErrInvalidMoney)
	}

	negative := strings.HasPrefix(raw, "-")
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "+"), "-")

	majorPart, minorPart, _ := strings.Cut(raw, ".")
	if majorPart == "" {
		majorPart = "0"
	}
	if len(majorPart) > _maxMajorDigits {
		return 0, fmt.Errorf("%w: %q is too large", ErrInvalidMoney, s)
	}
	if len(minorPart) > 2 {
		return 0, fmt.Errorf("%w: %q has more than 2 decimal places", ErrInvalidMoney, s)
	}
	for len(minorPart) < 2 {
		minorPart += "0"
	}
	if !isDigits(majorPart) || !isDigits(minorPart) {
		return 0, fmt.Errorf("%w: %q is not a decimal number", ErrInvalidMoney, s)
	}

	major, err := strconv.ParseInt(majorPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidMoney, s)
	}
	minor, err := strconv.ParseInt(minorPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidMoney, s)
	}

	total := major*_minorUnits + minor
	if negative {
		total = -total
	}

	return Money(total), nil
}

// String renders the exact decimal text, e.g. "10.00" — the form sent to
// Postgres (cast to numeric) and to JSON clients.
func (m Money) String() string {
	value := int64(m)

	sign := ""
	if value < 0 {
		sign = "-"
		// math.MinInt64 has no positive counterpart; guard before negating.
		if value == math.MinInt64 {
			return "-92233720368547758.08"
		}
		value = -value
	}

	return fmt.Sprintf("%s%d.%02d", sign, value/_minorUnits, value%_minorUnits)
}

// Mul multiplies by a unit count (price x quantity), staying in minor units.
func (m Money) Mul(quantity int) Money { return m * Money(quantity) }

// MarshalJSON emits money as a JSON string ("10.00"), never a JSON number:
// a number would invite float parsing on the client side.
func (m Money) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(m.String())
	if err != nil {
		return nil, fmt.Errorf("Money.MarshalJSON: %w", err)
	}

	return data, nil
}

// UnmarshalJSON accepts both a JSON string ("10.00") and a JSON number (10.00).
// The number is read as raw text and parsed exactly, so it never round-trips
// through float64.
func (m *Money) UnmarshalJSON(data []byte) error {
	text := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if text == "null" {
		return fmt.Errorf("%w: null", ErrInvalidMoney)
	}

	parsed, err := ParseMoney(text)
	if err != nil {
		return err
	}
	*m = parsed

	return nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

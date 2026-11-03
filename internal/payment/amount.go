package payment

import (
	"errors"
	"strconv"
	"strings"
)

var ErrInvalidAmount = errors.New("AMOUNT_OUT_OF_RANGE")

const MaxAmountCents int64 = 999999999

func ParseCents(value string) (int64, error) {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "eE+-") {
		return 0, ErrInvalidAmount
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, ErrInvalidAmount
	}
	for _, r := range parts[0] {
		if r < '0' || r > '9' {
			return 0, ErrInvalidAmount
		}
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) < 1 || len(fraction) > 2 {
			return 0, ErrInvalidAmount
		}
		for _, r := range fraction {
			if r < '0' || r > '9' {
				return 0, ErrInvalidAmount
			}
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > MaxAmountCents/100 {
		return 0, ErrInvalidAmount
	}
	cents := whole * 100
	if len(fraction) == 1 {
		fraction += "0"
	}
	if fraction != "" {
		n, err := strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, ErrInvalidAmount
		}
		cents += n
	}
	if cents < 1 || cents > MaxAmountCents {
		return 0, ErrInvalidAmount
	}
	return cents, nil
}

func FormatMoney(cents int64) string {
	return strconv.FormatInt(cents/100, 10) + "." + twoDigits(cents%100)
}
func twoDigits(value int64) string {
	if value < 10 {
		return "0" + strconv.FormatInt(value, 10)
	}
	return strconv.FormatInt(value, 10)
}

func AllocateAmount(original int64, direction string, limit, min, max int, occupied map[int64]struct{}) (int64, error) {
	if original < 1 || original > MaxAmountCents || limit < 0 || limit > 999 || min < 0 || max < min || (direction != "up" && direction != "down") {
		return 0, ErrInvalidAmount
	}
	sign := int64(-1)
	if direction == "up" {
		sign = 1
	}
	for delta := int64(0); delta <= int64(limit); delta++ {
		candidate := original + sign*delta
		if candidate < 1 || candidate > MaxAmountCents || candidate < int64(min) || candidate > int64(max) {
			continue
		}
		if _, exists := occupied[candidate]; !exists {
			return candidate, nil
		}
	}
	return 0, errors.New("AMOUNT_CAPACITY_EXHAUSTED")
}

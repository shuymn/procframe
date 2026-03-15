package config

// RedactedPlaceholder is the canonical replacement value for masked strings.
const RedactedPlaceholder = "[REDACTED]"

// RedactIfSet returns replacement when value is non-zero; otherwise it returns value.
func RedactIfSet[T comparable](value, replacement T) T {
	var zero T
	if value == zero {
		return value
	}
	return replacement
}

// RedactBytes returns a masked byte slice for non-empty input.
func RedactBytes(value []byte) []byte {
	if len(value) == 0 {
		return value
	}
	return []byte(RedactedPlaceholder)
}

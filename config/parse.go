package config

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// EnumMapping maps a string enum value to its numeric representation.
type EnumMapping struct {
	Name   string
	Number int32
}

// ParseString parses a string scalar from text.
func ParseString(raw string) (string, error) {
	return raw, nil
}

// ParseBytes parses a bytes scalar from base64 text.
func ParseBytes(raw string) ([]byte, error) {
	v, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("parse bytes: %w", err)
	}
	return v, nil
}

// ParseBool parses a bool scalar.
func ParseBool(raw string) (bool, error) {
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("parse bool: %w", err)
	}
	return v, nil
}

// ParseInt32 parses an int32 scalar.
func ParseInt32(raw string) (int32, error) {
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse int32: %w", err)
	}
	return int32(v), nil
}

// ParseInt64 parses an int64 scalar.
func ParseInt64(raw string) (int64, error) {
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse int64: %w", err)
	}
	return v, nil
}

// ParseUint32 parses a uint32 scalar.
func ParseUint32(raw string) (uint32, error) {
	v, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse uint32: %w", err)
	}
	return uint32(v), nil
}

// ParseUint64 parses a uint64 scalar.
func ParseUint64(raw string) (uint64, error) {
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse uint64: %w", err)
	}
	return v, nil
}

// ParseFloat32 parses a float32 scalar.
func ParseFloat32(raw string) (float32, error) {
	v, err := strconv.ParseFloat(raw, 32)
	if err != nil {
		return 0, fmt.Errorf("parse float32: %w", err)
	}
	if v > math.MaxFloat32 || v < -math.MaxFloat32 {
		return 0, fmt.Errorf("parse float32: value %q overflows float32", raw)
	}
	return float32(v), nil
}

// ParseFloat64 parses a float64 scalar.
func ParseFloat64(raw string) (float64, error) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("parse float64: %w", err)
	}
	return v, nil
}

// ParseEnum parses a case-insensitive enum value from mappings.
func ParseEnum(raw string, mappings []*EnumMapping, typeName string) (int32, error) {
	lower := strings.ToLower(raw)
	valid := make([]string, 0, len(mappings))
	for _, m := range mappings {
		if m == nil {
			continue
		}
		if strings.ToLower(m.Name) == lower {
			return m.Number, nil
		}
		valid = append(valid, m.Name)
	}
	if typeName == "" {
		typeName = "enum"
	}
	return 0, fmt.Errorf("invalid %s value %q (valid: %s)", typeName, raw, strings.Join(valid, ", "))
}

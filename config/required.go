package config

import "fmt"

// ValidateRequired returns an error when a required field was never set.
func ValidateRequired(fieldName string, present bool) error {
	if present {
		return nil
	}
	if fieldName == "" {
		return fmt.Errorf("required field is not set")
	}
	return fmt.Errorf("%s is required", fieldName)
}

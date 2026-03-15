package codegen

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/shuymn/procframe/transport/cli"
)

// enumValueInfo holds the proto-level information about an enum value.
type enumValueInfo struct {
	ProtoName string
	Number    int32
}

// fieldToFlagName converts a proto snake_case field name to a CLI
// kebab-case flag name (e.g. "log_level" → "log-level").
func fieldToFlagName(name string) string {
	return strings.ReplaceAll(name, "_", "-")
}

// enumCLIValues strips the type-derived prefix from enum value names,
// lowercases the remainder, and detects collisions.
func enumCLIValues(typeName string, values []enumValueInfo) ([]cli.EnumMapping, error) {
	prefix := camelToUpperSnake(typeName) + "_"
	mappings := make([]cli.EnumMapping, 0, len(values))
	seen := make(map[string]string, len(values))

	for _, v := range values {
		if v.Number == 0 {
			continue
		}
		stripped := v.ProtoName
		if after, ok := strings.CutPrefix(stripped, prefix); ok {
			stripped = after
		}
		cliVal := strings.ToLower(stripped)

		if prev, dup := seen[cliVal]; dup {
			return nil, fmt.Errorf(
				"enum %s: stripped value %q collides between %s and %s",
				typeName, cliVal, prev, v.ProtoName,
			)
		}
		seen[cliVal] = v.ProtoName
		mappings = append(mappings, cli.EnumMapping{CLIValue: cliVal, Number: v.Number})
	}
	return mappings, nil
}

// camelToUpperSnake converts CamelCase to UPPER_SNAKE_CASE.
// Handles runs of uppercase letters (e.g. "HTTPStatus" → "HTTP_STATUS").
func camelToUpperSnake(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			if unicode.IsLower(prev) {
				b.WriteByte('_')
			} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

package sshconfig

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidateAlias reports whether an SSH host alias can be written and parsed
// safely by vpsm-managed config handling.
func ValidateAlias(alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	for _, r := range alias {
		switch {
		case unicode.IsSpace(r):
			return fmt.Errorf("alias must not contain whitespace")
		case r == '*' || r == '?':
			return fmt.Errorf("alias must not contain wildcard characters")
		case r == '!':
			return fmt.Errorf("alias must not contain negation markers")
		case r == '"' || r == '\'':
			return fmt.Errorf("alias must not contain quotes")
		}
	}

	return nil
}

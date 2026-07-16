package typecfg

import (
	"fmt"
	"reflect"
)

// redactedMarker replaces field values in error messages when the field is
// tagged secret:"true".
const redactedMarker = "***REDACTED***"

func isSecret(field reflect.StructField) bool {
	return field.Tag.Get("secret") == "true"
}

// displayValue returns v formatted for an error message, or redactedMarker
// when secret is true.
func displayValue(secret bool, v any) string {
	if secret {
		return redactedMarker
	}
	return fmt.Sprintf("%v", v)
}

// displayQuoted returns a %q-style quoted string for error messages.
func displayQuoted(secret bool, s string) string {
	if secret {
		return fmt.Sprintf("%q", redactedMarker)
	}
	return fmt.Sprintf("%q", s)
}

// bindTypeReason is the FieldError.Reason for a bind-time conversion failure.
// When secret, the underlying err (which often embeds the raw input) is
// discarded so the value cannot leak.
func bindTypeReason(secret bool, target reflect.Type, err error) string {
	if !secret {
		return err.Error()
	}
	return fmt.Sprintf("cannot convert %s to %s", redactedMarker, target)
}

// defaultTagReason is the FieldError.Reason for an invalid default tag value.
func defaultTagReason(secret bool, def string, err error) string {
	if !secret {
		return fmt.Sprintf("invalid default value %q: %v", def, err)
	}
	return fmt.Sprintf("invalid default value %s: type conversion failed", redactedMarker)
}

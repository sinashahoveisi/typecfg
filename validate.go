package typecfg

import (
	"fmt"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// Validator can be implemented by any config struct (or nested struct) to
// add validation beyond what struct tags express, e.g. cross-field checks
// like "StartPort must be < EndPort".
type Validator interface {
	Validate() error
}

// validate walks the struct applying `validate:"..."` tag rules and
// collects every violation instead of stopping at the first one.
func validate(dst any, setFields map[string]struct{}) []*FieldError {
	v := reflect.ValueOf(dst).Elem()
	var errs []*FieldError
	validateStruct(v, "", &errs, setFields)

	if validator, ok := dst.(Validator); ok {
		if err := validator.Validate(); err != nil {
			errs = append(errs, &FieldError{Field: "<struct>", Tag: "Validate()", Reason: err.Error()})
		}
	}
	return errs
}

func validateStruct(v reflect.Value, pathPrefix string, errs *[]*FieldError, setFields map[string]struct{}) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fv := v.Field(i)
		key := fieldKey(field)
		fieldPath := key
		if pathPrefix != "" {
			fieldPath = pathPrefix + "." + key
		}

		if fv.Kind() == reflect.Struct {
			validateStruct(fv, fieldPath, errs, setFields)
		} else if fv.Kind() == reflect.Ptr && !fv.IsNil() && fv.Elem().Kind() == reflect.Struct {
			validateStruct(fv.Elem(), fieldPath, errs, setFields)
		}

		rules, ok := field.Tag.Lookup("validate")
		if !ok {
			continue
		}
		for _, rule := range strings.Split(rules, ",") {
			rule = strings.TrimSpace(rule)
			if rule == "" {
				continue
			}
			name, arg, _ := strings.Cut(rule, "=")
			if err := applyRule(fv, name, arg, fieldPath, setFields); err != "" {
				*errs = append(*errs, &FieldError{
					Field:   fieldPath,
					Tag:     name,
					Sources: possibleSources(field, key),
					Reason:  err,
				})
			}
		}
	}
}

func possibleSources(field reflect.StructField, key string) []string {
	srcs := []string{"cfg:" + key}
	if env, ok := field.Tag.Lookup("env"); ok {
		srcs = append(srcs, "env:"+env)
	}
	return srcs
}

// applyRule returns a non-empty human-readable reason if the rule fails,
// or "" if it passes.
func applyRule(fv reflect.Value, name, arg, fieldPath string, setFields map[string]struct{}) string {
	switch name {
	case "required":
		if _, ok := setFields[fieldPath]; !ok {
			return "is required but was not set"
		}
	case "min":
		limit, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return fmt.Sprintf("invalid min=%q on tag", arg)
		}
		if !numericAtLeast(fv, limit) {
			return fmt.Sprintf("must be >= %v, got %v", arg, fv.Interface())
		}
	case "max":
		limit, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return fmt.Sprintf("invalid max=%q on tag", arg)
		}
		if !numericAtMost(fv, limit) {
			return fmt.Sprintf("must be <= %v, got %v", arg, fv.Interface())
		}
	case "gt":
		limit, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return fmt.Sprintf("invalid gt=%q on tag", arg)
		}
		if !numericGreaterThan(fv, limit) {
			return fmt.Sprintf("must be > %v, got %v", arg, fv.Interface())
		}
	case "lt":
		limit, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return fmt.Sprintf("invalid lt=%q on tag", arg)
		}
		if !numericLessThan(fv, limit) {
			return fmt.Sprintf("must be < %v, got %v", arg, fv.Interface())
		}
	case "oneof":
		options := strings.Fields(arg)
		got := fmt.Sprintf("%v", fv.Interface())
		for _, o := range options {
			if o == got {
				return ""
			}
		}
		return fmt.Sprintf("must be one of %v, got %q", options, got)
	case "regexp":
		return applyRegexp(fv, arg)
	case "url":
		return applyURL(fv)
	case "email":
		return applyEmail(fv)
	}
	return ""
}

func requireString(fv reflect.Value, rule string) (string, string) {
	if fv.Kind() != reflect.String {
		return "", fmt.Sprintf("%s is only valid on string fields, got %s", rule, fv.Kind())
	}
	return fv.String(), ""
}

func applyRegexp(fv reflect.Value, pattern string) string {
	s, bad := requireString(fv, "regexp")
	if bad != "" {
		return bad
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Sprintf("invalid regexp pattern %q on field: %v", pattern, err)
	}
	if !re.MatchString(s) {
		return fmt.Sprintf("must match pattern %q, got %q", pattern, s)
	}
	return ""
}

func applyURL(fv reflect.Value) string {
	s, bad := requireString(fv, "url")
	if bad != "" {
		return bad
	}
	if _, err := url.ParseRequestURI(s); err != nil {
		return fmt.Sprintf("must be a valid URL, got %q: %v", s, err)
	}
	return ""
}

func applyEmail(fv reflect.Value) string {
	s, bad := requireString(fv, "email")
	if bad != "" {
		return bad
	}
	if _, err := mail.ParseAddress(s); err != nil {
		return fmt.Sprintf("must be a valid email address, got %q: %v", s, err)
	}
	return ""
}

func numericAtLeast(fv reflect.Value, limit float64) bool {
	f, ok := toFloat(fv)
	return !ok || f >= limit
}

func numericAtMost(fv reflect.Value, limit float64) bool {
	f, ok := toFloat(fv)
	return !ok || f <= limit
}

func numericGreaterThan(fv reflect.Value, limit float64) bool {
	f, ok := toFloat(fv)
	return !ok || f > limit
}

func numericLessThan(fv reflect.Value, limit float64) bool {
	f, ok := toFloat(fv)
	return !ok || f < limit
}

func toFloat(fv reflect.Value) (float64, bool) {
	switch fv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(fv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(fv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return fv.Float(), true
	case reflect.String:
		return float64(len(fv.String())), true // len-based min/max for strings
	default:
		return 0, false
	}
}

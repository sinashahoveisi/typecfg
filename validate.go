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
// data is the merged source map at the root (and nested maps when
// recursing); used only for "did you mean" suggestions on missing
// required fields.
func validate(dst any, setFields map[string]struct{}, data map[string]any) []*FieldError {
	v := reflect.ValueOf(dst).Elem()
	var errs []*FieldError
	validateStruct(v, "", &errs, setFields, data)

	if validator, ok := dst.(Validator); ok {
		if err := validator.Validate(); err != nil {
			errs = append(errs, &FieldError{Field: "<struct>", Tag: "Validate()", Reason: err.Error()})
		}
	}
	return errs
}

func validateStruct(v reflect.Value, pathPrefix string, errs *[]*FieldError, setFields map[string]struct{}, data map[string]any) {
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
			validateStruct(fv, fieldPath, errs, setFields, nestedDataMap(data, key))
		} else if fv.Kind() == reflect.Ptr && !fv.IsNil() && fv.Elem().Kind() == reflect.Struct {
			validateStruct(fv.Elem(), fieldPath, errs, setFields, nestedDataMap(data, key))
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
			if err := applyRule(fv, name, arg, fieldPath, key, setFields, data, isSecret(field)); err != "" {
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

func nestedDataMap(data map[string]any, key string) map[string]any {
	if data == nil {
		return nil
	}
	raw, ok := lookupCaseInsensitive(data, key)
	if !ok {
		return nil
	}
	if m, ok := raw.(map[string]any); ok {
		return m
	}
	return nil
}

func possibleSources(field reflect.StructField, key string) []string {
	srcs := []string{"cfg:" + key}
	if env, ok := field.Tag.Lookup("env"); ok {
		srcs = append(srcs, "env:"+env)
	}
	return srcs
}

// applyRule returns a non-empty human-readable reason if the rule fails,
// or "" if it passes. When secret is true, the field's actual value is
// replaced with redactedMarker; rule args (limits, patterns, oneof options)
// are never redacted.
func applyRule(fv reflect.Value, name, arg, fieldPath, key string, setFields map[string]struct{}, data map[string]any, secret bool) string {
	switch name {
	case "required":
		if _, ok := setFields[fieldPath]; !ok {
			msg := "is required but was not set"
			if suggestion := uniqueCloseKey(key, data); suggestion != "" {
				msg += fmt.Sprintf(` (did you mean %q? a similar key was found in the source)`, suggestion)
			}
			return msg
		}
	case "min":
		limit, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return fmt.Sprintf("invalid min=%q on tag", arg)
		}
		if !numericAtLeast(fv, limit) {
			return fmt.Sprintf("must be >= %v, got %v", arg, displayValue(secret, fv.Interface()))
		}
	case "max":
		limit, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return fmt.Sprintf("invalid max=%q on tag", arg)
		}
		if !numericAtMost(fv, limit) {
			return fmt.Sprintf("must be <= %v, got %v", arg, displayValue(secret, fv.Interface()))
		}
	case "gt":
		limit, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return fmt.Sprintf("invalid gt=%q on tag", arg)
		}
		if !numericGreaterThan(fv, limit) {
			return fmt.Sprintf("must be > %v, got %v", arg, displayValue(secret, fv.Interface()))
		}
	case "lt":
		limit, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return fmt.Sprintf("invalid lt=%q on tag", arg)
		}
		if !numericLessThan(fv, limit) {
			return fmt.Sprintf("must be < %v, got %v", arg, displayValue(secret, fv.Interface()))
		}
	case "oneof":
		options := strings.Fields(arg)
		got := fmt.Sprintf("%v", fv.Interface())
		for _, o := range options {
			if o == got {
				return ""
			}
		}
		msg := fmt.Sprintf("must be one of %v, got %s", options, displayQuoted(secret, got))
		// Suggestions can leak partial value info; never suggest for secrets.
		if !secret {
			if suggestion := uniqueCloseString(got, options); suggestion != "" {
				msg += fmt.Sprintf(` (did you mean %q?)`, suggestion)
			}
		}
		return msg
	case "regexp":
		return applyRegexp(fv, arg, secret)
	case "url":
		return applyURL(fv, secret)
	case "email":
		return applyEmail(fv, secret)
	}
	return ""
}

func requireString(fv reflect.Value, rule string) (string, string) {
	if fv.Kind() != reflect.String {
		return "", fmt.Sprintf("%s is only valid on string fields, got %s", rule, fv.Kind())
	}
	return fv.String(), ""
}

func applyRegexp(fv reflect.Value, pattern string, secret bool) string {
	s, bad := requireString(fv, "regexp")
	if bad != "" {
		return bad
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Sprintf("invalid regexp pattern %q on field: %v", pattern, err)
	}
	if !re.MatchString(s) {
		return fmt.Sprintf("must match pattern %q, got %s", pattern, displayQuoted(secret, s))
	}
	return ""
}

func applyURL(fv reflect.Value, secret bool) string {
	s, bad := requireString(fv, "url")
	if bad != "" {
		return bad
	}
	if _, err := url.ParseRequestURI(s); err != nil {
		if secret {
			// url.Error often embeds the input string; drop it when secret.
			return fmt.Sprintf("must be a valid URL, got %s", displayQuoted(true, s))
		}
		return fmt.Sprintf("must be a valid URL, got %q: %v", s, err)
	}
	return ""
}

func applyEmail(fv reflect.Value, secret bool) string {
	s, bad := requireString(fv, "email")
	if bad != "" {
		return bad
	}
	if _, err := mail.ParseAddress(s); err != nil {
		if secret {
			// mail.ParseAddress errors may embed the address; drop them.
			return fmt.Sprintf("must be a valid email address, got %s", displayQuoted(true, s))
		}
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

// uniqueCloseKey returns the single sibling key in data within edit
// distance <= 2 of expected, or "" if zero or multiple candidates match.
func uniqueCloseKey(expected string, data map[string]any) string {
	if data == nil {
		return ""
	}
	candidates := make([]string, 0, len(data))
	for k := range data {
		candidates = append(candidates, k)
	}
	return uniqueCloseString(expected, candidates)
}

func uniqueCloseString(target string, candidates []string) string {
	var match string
	found := 0
	for _, c := range candidates {
		if c == target {
			continue
		}
		if levenshtein(target, c) <= 2 {
			found++
			match = c
			if found > 1 {
				return ""
			}
		}
	}
	if found == 1 {
		return match
	}
	return ""
}

// levenshtein returns the classic edit distance between a and b.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = del
			if ins < curr[j] {
				curr[j] = ins
			}
			if sub < curr[j] {
				curr[j] = sub
			}
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

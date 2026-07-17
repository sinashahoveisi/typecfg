package typecfg

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Lookup finds key in data with the same case-insensitive rules as bind/validate.
func Lookup(data map[string]any, key string) (any, bool) {
	return lookupCaseInsensitive(data, key)
}

// NestedRaw returns the nested map for key, or nil — same as validate's recursion.
func NestedRaw(data map[string]any, key string) map[string]any {
	return nestedDataMap(data, key)
}

// AsString formats raw the same way the reflection binder does before setScalar.
func AsString(raw any) string {
	return fmt.Sprintf("%v", raw)
}

// FieldSources builds FieldError.Sources for a validate failure.
func FieldSources(key, envTag string) []string {
	srcs := []string{"cfg:" + key}
	if envTag != "" {
		srcs = append(srcs, "env:"+envTag)
	}
	return srcs
}

// NewBindTypeError builds a bind-time type FieldError matching bind.go.
func NewBindTypeError(fieldPath, key, typeName string, secret bool, err error) *FieldError {
	return &FieldError{
		Field:   fieldPath,
		Tag:     "type",
		Sources: []string{"cfg:" + key},
		Reason:  bindTypeReasonNamed(secret, typeName, err),
	}
}

// NewDefaultError builds a default-tag FieldError matching bind.go.
func NewDefaultError(fieldPath, def string, secret bool, err error) *FieldError {
	return &FieldError{
		Field:  fieldPath,
		Tag:    "default",
		Reason: defaultTagReason(secret, def, err),
	}
}

func bindTypeReasonNamed(secret bool, typeName string, err error) string {
	if !secret {
		return err.Error()
	}
	return fmt.Sprintf("cannot convert %s to %s", redactedMarker, typeName)
}

// CheckRule applies one validate rule to value, matching applyRule.
func CheckRule(name, arg, fieldPath, key string, value any, setFields map[string]struct{}, data map[string]any, secret bool) string {
	return applyRule(reflect.ValueOf(value), name, arg, fieldPath, key, setFields, data, secret)
}

// ParseBool parses s like the reflection binder.
func ParseBool(s string) (bool, error) { return strconv.ParseBool(s) }

// ParseInt parses s like the reflection binder (bitSize for overflow checks).
func ParseInt(s string, bitSize int) (int64, error) {
	return strconv.ParseInt(s, 10, bitSize)
}

// ParseUint parses s like the reflection binder.
func ParseUint(s string, bitSize int) (uint64, error) {
	return strconv.ParseUint(s, 10, bitSize)
}

// ParseFloat parses s like the reflection binder.
func ParseFloat(s string, bitSize int) (float64, error) {
	return strconv.ParseFloat(s, bitSize)
}

// ParseDuration parses s like the reflection binder for time.Duration fields.
func ParseDuration(s string) (time.Duration, error) { return time.ParseDuration(s) }

// ParseTime parses s with layout (empty means RFC3339), matching setTime.
func ParseTime(s, layout string) (time.Time, error) {
	if layout == "" {
		layout = time.RFC3339
	}
	t, err := time.Parse(layout, s)
	if err != nil {
		if layout == time.RFC3339 {
			return time.Time{}, fmt.Errorf(`expected RFC3339 (e.g. "2026-01-01T00:00:00Z"), got %q`, s)
		}
		return time.Time{}, fmt.Errorf("expected layout %q, got %q", layout, s)
	}
	return t, nil
}

// CoerceStringMap converts raw to map[string]string like setStringMap.
func CoerceStringMap(raw any) (map[string]string, error) {
	switch v := raw.(type) {
	case map[string]any:
		m := make(map[string]string, len(v))
		for k, val := range v {
			m[k] = fmt.Sprintf("%v", val)
		}
		return m, nil
	case map[string]string:
		m := make(map[string]string, len(v))
		for k, val := range v {
			m[k] = val
		}
		return m, nil
	case string:
		return stringMapFromJSON(v)
	default:
		return stringMapFromJSON(fmt.Sprintf("%v", raw))
	}
}

func stringMapFromJSON(s string) (map[string]string, error) {
	m := map[string]string{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("invalid JSON for map[string]string %q: %v", s, err)
	}
	return m, nil
}

// CoerceStringSlice converts raw to []string like setSliceValue.
func CoerceStringSlice(raw any) ([]string, error) {
	if s, ok := raw.(string); ok {
		return stringSliceFromComma(s), nil
	}
	elems, err := rawToStringElems(raw)
	if err != nil {
		return nil, err
	}
	return elems, nil
}

func stringSliceFromComma(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// CoerceIntSlice converts raw to []int like setSliceValue for []int.
func CoerceIntSlice(raw any) ([]int, error) {
	return coerceSignedSlice[int](raw, strconv.IntSize)
}

// CoerceInt8Slice converts raw to []int8.
func CoerceInt8Slice(raw any) ([]int8, error) {
	return coerceSignedSlice[int8](raw, 8)
}

// CoerceInt16Slice converts raw to []int16.
func CoerceInt16Slice(raw any) ([]int16, error) {
	return coerceSignedSlice[int16](raw, 16)
}

// CoerceInt32Slice converts raw to []int32.
func CoerceInt32Slice(raw any) ([]int32, error) {
	return coerceSignedSlice[int32](raw, 32)
}

// CoerceInt64Slice converts raw to []int64.
func CoerceInt64Slice(raw any) ([]int64, error) {
	return coerceSignedSlice[int64](raw, 64)
}

// CoerceUintSlice converts raw to []uint.
func CoerceUintSlice(raw any) ([]uint, error) {
	return coerceUnsignedSlice[uint](raw, strconv.IntSize)
}

// CoerceUint8Slice converts raw to []uint8.
func CoerceUint8Slice(raw any) ([]uint8, error) {
	return coerceUnsignedSlice[uint8](raw, 8)
}

// CoerceUint16Slice converts raw to []uint16.
func CoerceUint16Slice(raw any) ([]uint16, error) {
	return coerceUnsignedSlice[uint16](raw, 16)
}

// CoerceUint32Slice converts raw to []uint32.
func CoerceUint32Slice(raw any) ([]uint32, error) {
	return coerceUnsignedSlice[uint32](raw, 32)
}

// CoerceUint64Slice converts raw to []uint64.
func CoerceUint64Slice(raw any) ([]uint64, error) {
	return coerceUnsignedSlice[uint64](raw, 64)
}

// CoerceFloat32Slice converts raw to []float32.
func CoerceFloat32Slice(raw any) ([]float32, error) {
	return coerceFloatSlice[float32](raw, 32)
}

// CoerceFloat64Slice converts raw to []float64.
func CoerceFloat64Slice(raw any) ([]float64, error) {
	return coerceFloatSlice[float64](raw, 64)
}

type signedInt interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type unsignedInt interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

type floatNum interface {
	~float32 | ~float64
}

func coerceSignedSlice[T signedInt](raw any, bitSize int) ([]T, error) {
	texts, err := rawToStringElems(raw)
	if err != nil {
		return nil, err
	}
	typeName := reflect.TypeOf((*T)(nil)).Elem().String()
	out := make([]T, len(texts))
	for i, p := range texts {
		n, err := strconv.ParseInt(p, 10, bitSize)
		if err != nil {
			return nil, fmt.Errorf("element %d (%q) is not a valid %s", i, p, typeName)
		}
		out[i] = T(n)
	}
	return out, nil
}

func coerceUnsignedSlice[T unsignedInt](raw any, bitSize int) ([]T, error) {
	texts, err := rawToStringElems(raw)
	if err != nil {
		return nil, err
	}
	typeName := reflect.TypeOf((*T)(nil)).Elem().String()
	out := make([]T, len(texts))
	for i, p := range texts {
		n, err := strconv.ParseUint(p, 10, bitSize)
		if err != nil {
			return nil, fmt.Errorf("element %d (%q) is not a valid %s", i, p, typeName)
		}
		out[i] = T(n)
	}
	return out, nil
}

func coerceFloatSlice[T floatNum](raw any, bitSize int) ([]T, error) {
	texts, err := rawToStringElems(raw)
	if err != nil {
		return nil, err
	}
	typeName := reflect.TypeOf((*T)(nil)).Elem().String()
	out := make([]T, len(texts))
	for i, p := range texts {
		n, err := strconv.ParseFloat(p, bitSize)
		if err != nil {
			return nil, fmt.Errorf("element %d (%q) is not a valid %s", i, p, typeName)
		}
		out[i] = T(n)
	}
	return out, nil
}

// rawToStringElems mirrors setSlice / setSliceValue element extraction.
func rawToStringElems(raw any) ([]string, error) {
	if s, ok := raw.(string); ok {
		if strings.TrimSpace(s) == "" {
			return []string{}, nil
		}
		parts := strings.Split(s, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}
		return parts, nil
	}
	rv := reflect.ValueOf(raw)
	if !rv.IsValid() {
		return []string{}, nil
	}
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		s := fmt.Sprintf("%v", raw)
		if strings.TrimSpace(s) == "" {
			return []string{}, nil
		}
		parts := strings.Split(s, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}
		return parts, nil
	}
	out := make([]string, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = strings.TrimSpace(fmt.Sprintf("%v", rv.Index(i).Interface()))
	}
	return out, nil
}

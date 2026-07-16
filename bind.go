package typecfg

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// bind walks dst (a pointer to struct) and fills it from the nested map,
// using `cfg:"name"` tags to find the source key (falling back to the
// lowercased field name) and `default:"..."` for fallback values.
// It returns FieldErrors for type-conversion failures; required/oneof/min/
// max checks happen separately in validate.go so all errors can be
// collected together.
func bind(dst any, data map[string]any) ([]*FieldError, map[string]struct{}) {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return []*FieldError{{Field: "<root>", Reason: "target must be a pointer to struct"}}, nil
	}
	var errs []*FieldError
	setFields := make(map[string]struct{})
	bindStruct(v.Elem(), data, "", &errs, setFields)
	return errs, setFields
}

func bindStruct(v reflect.Value, data map[string]any, pathPrefix string, errs *[]*FieldError, setFields map[string]struct{}) {
	t := v.Type()
	timeType := reflect.TypeOf(time.Time{})
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

		raw, present := lookupCaseInsensitive(data, key)
		layout := field.Tag.Get("layout")

		if fv.Kind() == reflect.Struct && fv.Type() != timeType {
			var nested map[string]any
			if present {
				if m, ok := raw.(map[string]any); ok {
					nested = m
				}
			}
			bindStruct(fv, nested, fieldPath, errs, setFields)
			continue
		}

		// Exclude *time.Time so it falls through to scalar binding.
		if fv.Kind() == reflect.Ptr && fv.Type().Elem().Kind() == reflect.Struct && fv.Type().Elem() != timeType {
			var nested map[string]any
			if present {
				if m, ok := raw.(map[string]any); ok {
					nested = m
				}
			}
			if nested != nil {
				elem := reflect.New(fv.Type().Elem())
				bindStruct(elem.Elem(), nested, fieldPath, errs, setFields)
				fv.Set(elem)
			}
			continue
		}

		if !present {
			if def, ok := field.Tag.Lookup("default"); ok {
				if err := setScalar(fv, def, layout); err != nil {
					*errs = append(*errs, &FieldError{
						Field: fieldPath, Tag: "default",
						Reason: fmt.Sprintf("invalid default value %q: %v", def, err),
					})
				} else {
					setFields[fieldPath] = struct{}{}
				}
			}
			continue
		}

		// map[string]string from nested maps (YAML/JSON) must not be
		// fmt.Sprintf'd; coerce values directly. Flat strings (env) go
		// through JSON unmarshal inside setStringMap.
		if isStringStringMap(fv.Type()) {
			if err := setStringMap(fv, raw); err != nil {
				*errs = append(*errs, &FieldError{
					Field:   fieldPath,
					Tag:     "type",
					Sources: []string{"cfg:" + key},
					Reason:  err.Error(),
				})
			} else {
				setFields[fieldPath] = struct{}{}
			}
			continue
		}

		if fv.Kind() == reflect.Slice {
			if err := setSliceValue(fv, raw); err != nil {
				*errs = append(*errs, &FieldError{
					Field:   fieldPath,
					Tag:     "type",
					Sources: []string{"cfg:" + key},
					Reason:  err.Error(),
				})
			} else {
				setFields[fieldPath] = struct{}{}
			}
			continue
		}

		strVal := fmt.Sprintf("%v", raw)
		if err := setScalar(fv, strVal, layout); err != nil {
			*errs = append(*errs, &FieldError{
				Field:   fieldPath,
				Tag:     "type",
				Sources: []string{"cfg:" + key},
				Reason:  err.Error(),
			})
		} else {
			setFields[fieldPath] = struct{}{}
		}
	}
}

func fieldKey(field reflect.StructField) string {
	if tag, ok := field.Tag.Lookup("cfg"); ok && tag != "" {
		name, _, _ := strings.Cut(tag, ",")
		if name != "" {
			return name
		}
	}
	return strings.ToLower(field.Name)
}

func lookupCaseInsensitive(data map[string]any, key string) (any, bool) {
	if data == nil {
		return nil, false
	}
	if v, ok := data[key]; ok {
		return v, true
	}
	lower := strings.ToLower(key)
	for k, v := range data {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return nil, false
}

func setScalar(fv reflect.Value, s string, layout string) error {
	if fv.Type() == reflect.TypeOf(time.Duration(0)) {
		d, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		fv.SetInt(int64(d))
		return nil
	}
	if fv.Type() == reflect.TypeOf(time.Time{}) {
		return setTime(fv, s, layout)
	}
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(s)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return err
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		fv.SetFloat(n)
	case reflect.Slice:
		return setSlice(fv, s)
	case reflect.Map:
		if isStringStringMap(fv.Type()) {
			return setStringMapFromJSON(fv, s)
		}
		return fmt.Errorf("unsupported map type %s", fv.Type())
	case reflect.Ptr:
		elem := reflect.New(fv.Type().Elem())
		if err := setScalar(elem.Elem(), s, layout); err != nil {
			return err
		}
		fv.Set(elem)
	default:
		return fmt.Errorf("unsupported field type %s", fv.Type())
	}
	return nil
}

func setTime(fv reflect.Value, s string, layout string) error {
	if layout == "" {
		layout = time.RFC3339
	}
	t, err := time.Parse(layout, s)
	if err != nil {
		if layout == time.RFC3339 {
			return fmt.Errorf(`expected RFC3339 (e.g. "2026-01-01T00:00:00Z"), got %q`, s)
		}
		return fmt.Errorf("expected layout %q, got %q", layout, s)
	}
	fv.Set(reflect.ValueOf(t))
	return nil
}

func setSlice(fv reflect.Value, s string) error {
	elemType := fv.Type().Elem()
	elemKind := elemType.Kind()

	if strings.TrimSpace(s) == "" {
		fv.Set(reflect.MakeSlice(fv.Type(), 0, 0))
		return nil
	}

	parts := strings.Split(s, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}

	if elemKind == reflect.String {
		fv.Set(reflect.ValueOf(parts))
		return nil
	}

	slice := reflect.MakeSlice(fv.Type(), len(parts), len(parts))
	for i, p := range parts {
		elem := slice.Index(i)
		switch elemKind {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(p, 10, elemType.Bits())
			if err != nil {
				return fmt.Errorf("element %d (%q) is not a valid %s", i, p, elemType)
			}
			elem.SetInt(n)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			n, err := strconv.ParseUint(p, 10, elemType.Bits())
			if err != nil {
				return fmt.Errorf("element %d (%q) is not a valid %s", i, p, elemType)
			}
			elem.SetUint(n)
		case reflect.Float32, reflect.Float64:
			n, err := strconv.ParseFloat(p, elemType.Bits())
			if err != nil {
				return fmt.Errorf("element %d (%q) is not a valid %s", i, p, elemType)
			}
			elem.SetFloat(n)
		default:
			return fmt.Errorf("unsupported slice element type %s", elemType)
		}
	}
	fv.Set(slice)
	return nil
}

func setSliceValue(fv reflect.Value, raw any) error {
	if s, ok := raw.(string); ok {
		return setSlice(fv, s)
	}

	rv := reflect.ValueOf(raw)
	if !rv.IsValid() {
		return setSlice(fv, "")
	}
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return setSlice(fv, fmt.Sprintf("%v", raw))
	}

	elemType := fv.Type().Elem()
	elemKind := elemType.Kind()
	slice := reflect.MakeSlice(fv.Type(), rv.Len(), rv.Len())
	for i := 0; i < rv.Len(); i++ {
		rawElem := rv.Index(i).Interface()
		text := strings.TrimSpace(fmt.Sprintf("%v", rawElem))
		elem := slice.Index(i)
		switch elemKind {
		case reflect.String:
			elem.SetString(text)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(text, 10, elemType.Bits())
			if err != nil {
				return fmt.Errorf("element %d (%q) is not a valid %s", i, text, elemType)
			}
			elem.SetInt(n)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			n, err := strconv.ParseUint(text, 10, elemType.Bits())
			if err != nil {
				return fmt.Errorf("element %d (%q) is not a valid %s", i, text, elemType)
			}
			elem.SetUint(n)
		case reflect.Float32, reflect.Float64:
			n, err := strconv.ParseFloat(text, elemType.Bits())
			if err != nil {
				return fmt.Errorf("element %d (%q) is not a valid %s", i, text, elemType)
			}
			elem.SetFloat(n)
		default:
			return fmt.Errorf("unsupported slice element type %s", elemType)
		}
	}
	fv.Set(slice)
	return nil
}

func isStringStringMap(t reflect.Type) bool {
	return t.Kind() == reflect.Map &&
		t.Key().Kind() == reflect.String &&
		t.Elem().Kind() == reflect.String
}

func setStringMap(fv reflect.Value, raw any) error {
	switch v := raw.(type) {
	case map[string]any:
		m := make(map[string]string, len(v))
		for k, val := range v {
			m[k] = fmt.Sprintf("%v", val)
		}
		fv.Set(reflect.ValueOf(m))
		return nil
	case map[string]string:
		// Copy so callers cannot mutate the source map via the config.
		m := make(map[string]string, len(v))
		for k, val := range v {
			m[k] = val
		}
		fv.Set(reflect.ValueOf(m))
		return nil
	case string:
		return setStringMapFromJSON(fv, v)
	default:
		return setStringMapFromJSON(fv, fmt.Sprintf("%v", raw))
	}
}

func setStringMapFromJSON(fv reflect.Value, s string) error {
	m := map[string]string{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return fmt.Errorf("invalid JSON for map[string]string %q: %v", s, err)
	}
	fv.Set(reflect.ValueOf(m))
	return nil
}

package typecfg

import (
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

		if fv.Kind() == reflect.Struct && fv.Type() != reflect.TypeOf(time.Time{}) {
			var nested map[string]any
			if present {
				if m, ok := raw.(map[string]any); ok {
					nested = m
				}
			}
			bindStruct(fv, nested, fieldPath, errs, setFields)
			continue
		}

		if fv.Kind() == reflect.Ptr && fv.Type().Elem().Kind() == reflect.Struct {
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
				if err := setScalar(fv, def); err != nil {
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

		strVal := fmt.Sprintf("%v", raw)
		if err := setScalar(fv, strVal); err != nil {
			*errs = append(*errs, &FieldError{
				Field:   fieldPath,
				Tag:     "type",
				Sources: []string{"cfg:" + key},
				Reason:  fmt.Sprintf("cannot convert %q to %s: %v", strVal, fv.Type(), err),
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

func setScalar(fv reflect.Value, s string) error {
	if fv.Type() == reflect.TypeOf(time.Duration(0)) {
		d, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		fv.SetInt(int64(d))
		return nil
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
		if fv.Type().Elem().Kind() == reflect.String {
			parts := strings.Split(s, ",")
			for i, p := range parts {
				parts[i] = strings.TrimSpace(p)
			}
			fv.Set(reflect.ValueOf(parts))
			return nil
		}
		return fmt.Errorf("unsupported slice element type %s", fv.Type().Elem())
	case reflect.Ptr:
		elem := reflect.New(fv.Type().Elem())
		if err := setScalar(elem.Elem(), s); err != nil {
			return err
		}
		fv.Set(elem)
	default:
		return fmt.Errorf("unsupported field type %s", fv.Type())
	}
	return nil
}

package typecfg

import (
	"reflect"
	"time"
)

// FieldChange describes one leaf field that differs between two configs.
type FieldChange struct {
	Path string
	Old  any
	New  any
}

// Diff compares old and new by walking exported fields with the same
// dotted cfg-tag paths used by bind/validate. Only leaf fields appear
// in the result. Fields tagged secret:"true" report Path but replace
// both Old and New with redactedMarker.
//
// If old is nil (e.g. first-ever load), every leaf in new is reported
// with Old == nil.
func Diff[T any](old, new *T) []FieldChange {
	if new == nil {
		return nil
	}
	nv := reflect.ValueOf(new).Elem()
	var ov reflect.Value
	if old != nil {
		ov = reflect.ValueOf(old).Elem()
	}
	var out []FieldChange
	diffStruct(ov, nv, "", &out)
	return out
}

func diffStruct(oldV, newV reflect.Value, pathPrefix string, out *[]FieldChange) {
	if !newV.IsValid() || newV.Kind() != reflect.Struct {
		return
	}
	t := newV.Type()
	timeType := reflect.TypeOf(time.Time{})

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		key := fieldKey(field)
		path := key
		if pathPrefix != "" {
			path = pathPrefix + "." + key
		}

		nv := newV.Field(i)
		var ov reflect.Value
		if oldV.IsValid() && oldV.Kind() == reflect.Struct && i < oldV.NumField() {
			ov = oldV.Field(i)
		}

		// Nested struct (not time.Time).
		if nv.Kind() == reflect.Struct && nv.Type() != timeType {
			diffStruct(ov, nv, path, out)
			continue
		}

		// *Struct (not *time.Time): recurse into the pointed-to value.
		if nv.Kind() == reflect.Ptr && nv.Type().Elem().Kind() == reflect.Struct && nv.Type().Elem() != timeType {
			var oldElem, newElem reflect.Value
			if nv.IsNil() {
				if !ov.IsValid() || ov.IsNil() {
					continue
				}
				// Cleared pointer: treat as leaf change from old value to nil.
				appendLeafChange(field, path, ov.Elem().Interface(), nil, out)
				continue
			}
			newElem = nv.Elem()
			if ov.IsValid() && ov.Kind() == reflect.Ptr && !ov.IsNil() {
				oldElem = ov.Elem()
			}
			diffStruct(oldElem, newElem, path, out)
			continue
		}

		appendLeafDiff(field, path, ov, nv, out)
	}
}

func appendLeafDiff(field reflect.StructField, path string, ov, nv reflect.Value, out *[]FieldChange) {
	var oldAny any
	hasOld := ov.IsValid() && ov.CanInterface()
	if hasOld {
		oldAny = ov.Interface()
	}
	newAny := nv.Interface()

	if !hasOld {
		appendLeafChange(field, path, nil, newAny, out)
		return
	}
	if reflect.DeepEqual(oldAny, newAny) {
		return
	}
	appendLeafChange(field, path, oldAny, newAny, out)
}

func appendLeafChange(field reflect.StructField, path string, oldAny, newAny any, out *[]FieldChange) {
	if isSecret(field) {
		// Keep Path; never expose real values. When Old was nil (first load),
		// leave Old as nil per Diff contract; only redact New / both when set.
		oldOut := oldAny
		newOut := newAny
		if oldAny != nil {
			oldOut = redactedMarker
		}
		if newAny != nil {
			newOut = redactedMarker
		}
		*out = append(*out, FieldChange{Path: path, Old: oldOut, New: newOut})
		return
	}
	*out = append(*out, FieldChange{Path: path, Old: oldAny, New: newAny})
}

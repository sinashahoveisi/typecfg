package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
)

// Options configures typecfg-gen code generation for one named struct.
type Options struct {
	// TypeName is the exported struct type to generate a binder for.
	TypeName string
	// InputPath is the .go file that declares TypeName (parsed with go/parser).
	InputPath string
	// PackageName is the package clause written into the generated file.
	// When it is "typecfg", helpers are referenced without an import prefix.
	PackageName string
}

type fieldKind int

const (
	kindString fieldKind = iota
	kindBool
	kindInt
	kindInt8
	kindInt16
	kindInt32
	kindInt64
	kindUint
	kindUint8
	kindUint16
	kindUint32
	kindUint64
	kindFloat32
	kindFloat64
	kindDuration
	kindTime
	kindStringSlice
	kindIntSlice
	kindInt8Slice
	kindInt16Slice
	kindInt32Slice
	kindInt64Slice
	kindUintSlice
	kindUint8Slice
	kindUint16Slice
	kindUint32Slice
	kindUint64Slice
	kindFloat32Slice
	kindFloat64Slice
	kindStringMap
	kindStruct
)

type fieldInfo struct {
	GoName     string
	CfgKey     string
	AccessPath string // e.g. "Server.Port" for Go selector from root cfg
	FieldPath  string // e.g. "server.port" for setFields / FieldError
	Kind       fieldKind
	TypeName   string // reflect-style name for error messages
	Default    string
	HasDefault bool
	Validate   string
	Secret     bool
	Layout     string
	EnvTag     string
	Nested     []fieldInfo
}

// Generate parses the input file, locates the named struct, and returns
// generated Go source implementing GeneratedBinder for that type.
func Generate(opts Options) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, opts.InputPath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", opts.InputPath, err)
	}

	conf := types.Config{Importer: importer.Default()}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
	}
	pkg, err := conf.Check(f.Name.Name, fset, []*ast.File{f}, info)
	if err != nil {
		return nil, fmt.Errorf("type-check %s: %w", opts.InputPath, err)
	}

	obj := pkg.Scope().Lookup(opts.TypeName)
	if obj == nil {
		return nil, fmt.Errorf("type %q not found in %s", opts.TypeName, opts.InputPath)
	}
	tn, ok := obj.(*types.TypeName)
	if !ok {
		return nil, fmt.Errorf("%q is not a named type", opts.TypeName)
	}
	st, ok := tn.Type().Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("%q is not a struct type", opts.TypeName)
	}

	fields, err := walkStruct(st, "", "", opts.TypeName)
	if err != nil {
		return nil, err
	}

	return emit(opts, fields)
}

func walkStruct(st *types.Struct, accessPrefix, pathPrefix, typeContext string) ([]fieldInfo, error) {
	var out []fieldInfo
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		if f.Embedded() {
			return nil, fmt.Errorf("field %s.%s: embedded fields are not supported by typecfg-gen", typeContext, f.Name())
		}
		tag := st.Tag(i)
		cfgKey := cfgKeyFromTag(tag, f.Name())
		access := f.Name()
		if accessPrefix != "" {
			access = accessPrefix + "." + f.Name()
		}
		fieldPath := cfgKey
		if pathPrefix != "" {
			fieldPath = pathPrefix + "." + cfgKey
		}

		fi := fieldInfo{
			GoName:     f.Name(),
			CfgKey:     cfgKey,
			AccessPath: access,
			FieldPath:  fieldPath,
			Default:    structTagGet(tag, "default"),
			HasDefault: structTagLookup(tag, "default"),
			Validate:   structTagGet(tag, "validate"),
			Secret:     structTagGet(tag, "secret") == "true",
			Layout:     structTagGet(tag, "layout"),
			EnvTag:     structTagGet(tag, "env"),
		}

		ft := f.Type()
		if _, ok := ft.(*types.Pointer); ok {
			return nil, fmt.Errorf("field %s (%s): pointer field types are not supported by typecfg-gen (got %s)", fieldPath, access, ft)
		}

		if isTimeType(ft) {
			fi.Kind = kindTime
			fi.TypeName = "time.Time"
			out = append(out, fi)
			continue
		}
		if isDurationType(ft) {
			fi.Kind = kindDuration
			fi.TypeName = "time.Duration"
			out = append(out, fi)
			continue
		}

		switch u := ft.Underlying().(type) {
		case *types.Struct:
			nested, err := walkStruct(u, access, fieldPath, typeContext+"."+f.Name())
			if err != nil {
				return nil, err
			}
			fi.Kind = kindStruct
			fi.TypeName = ft.String()
			fi.Nested = nested
			out = append(out, fi)
		case *types.Basic:
			kind, typeName, err := basicKind(u, fieldPath, access)
			if err != nil {
				return nil, err
			}
			fi.Kind = kind
			fi.TypeName = typeName
			out = append(out, fi)
		case *types.Slice:
			kind, typeName, err := sliceKind(u, fieldPath, access)
			if err != nil {
				return nil, err
			}
			fi.Kind = kind
			fi.TypeName = typeName
			out = append(out, fi)
		case *types.Map:
			if u.Key().Underlying().String() == "string" && u.Elem().Underlying().String() == "string" {
				fi.Kind = kindStringMap
				fi.TypeName = "map[string]string"
				out = append(out, fi)
				continue
			}
			return nil, fmt.Errorf("field %s (%s): unsupported map type %s (only map[string]string is supported)", fieldPath, access, ft)
		default:
			return nil, fmt.Errorf("field %s (%s): unsupported field type %s", fieldPath, access, ft)
		}
	}
	return out, nil
}

func basicKind(b *types.Basic, fieldPath, access string) (fieldKind, string, error) {
	switch b.Kind() {
	case types.String:
		return kindString, "string", nil
	case types.Bool:
		return kindBool, "bool", nil
	case types.Int:
		return kindInt, "int", nil
	case types.Int8:
		return kindInt8, "int8", nil
	case types.Int16:
		return kindInt16, "int16", nil
	case types.Int32:
		return kindInt32, "int32", nil
	case types.Int64:
		return kindInt64, "int64", nil
	case types.Uint:
		return kindUint, "uint", nil
	case types.Uint8:
		return kindUint8, "uint8", nil
	case types.Uint16:
		return kindUint16, "uint16", nil
	case types.Uint32:
		return kindUint32, "uint32", nil
	case types.Uint64:
		return kindUint64, "uint64", nil
	case types.Float32:
		return kindFloat32, "float32", nil
	case types.Float64:
		return kindFloat64, "float64", nil
	default:
		return 0, "", fmt.Errorf("field %s (%s): unsupported basic type %s", fieldPath, access, b)
	}
}

func sliceKind(s *types.Slice, fieldPath, access string) (fieldKind, string, error) {
	elem, ok := s.Elem().Underlying().(*types.Basic)
	if !ok {
		return 0, "", fmt.Errorf("field %s (%s): unsupported slice element type %s", fieldPath, access, s.Elem())
	}
	switch elem.Kind() {
	case types.String:
		return kindStringSlice, "[]string", nil
	case types.Int:
		return kindIntSlice, "[]int", nil
	case types.Int8:
		return kindInt8Slice, "[]int8", nil
	case types.Int16:
		return kindInt16Slice, "[]int16", nil
	case types.Int32:
		return kindInt32Slice, "[]int32", nil
	case types.Int64:
		return kindInt64Slice, "[]int64", nil
	case types.Uint:
		return kindUintSlice, "[]uint", nil
	case types.Uint8:
		return kindUint8Slice, "[]uint8", nil
	case types.Uint16:
		return kindUint16Slice, "[]uint16", nil
	case types.Uint32:
		return kindUint32Slice, "[]uint32", nil
	case types.Uint64:
		return kindUint64Slice, "[]uint64", nil
	case types.Float32:
		return kindFloat32Slice, "[]float32", nil
	case types.Float64:
		return kindFloat64Slice, "[]float64", nil
	default:
		return 0, "", fmt.Errorf("field %s (%s): unsupported slice element type %s", fieldPath, access, s.Elem())
	}
}

func isTimeType(t types.Type) bool {
	n, ok := t.(*types.Named)
	if !ok {
		return false
	}
	return n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "time" && n.Obj().Name() == "Time"
}

func isDurationType(t types.Type) bool {
	n, ok := t.(*types.Named)
	if !ok {
		return false
	}
	return n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "time" && n.Obj().Name() == "Duration"
}

func cfgKeyFromTag(tag, fieldName string) string {
	if v, ok := reflectTagLookup(tag, "cfg"); ok && v != "" {
		name, _, _ := strings.Cut(v, ",")
		if name != "" {
			return name
		}
	}
	return strings.ToLower(fieldName)
}

func structTagGet(tag, key string) string {
	v, _ := reflectTagLookup(tag, key)
	return v
}

func structTagLookup(tag, key string) bool {
	_, ok := reflectTagLookup(tag, key)
	return ok
}

// reflectTagLookup parses a raw struct tag string (as from types.Struct.Tag).
func reflectTagLookup(tag, key string) (string, bool) {
	for tag != "" {
		i := 0
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		tag = tag[i:]
		if tag == "" {
			break
		}
		i = 0
		for i < len(tag) && tag[i] > ' ' && tag[i] != ':' && tag[i] != '"' && tag[i] != 0x7f {
			i++
		}
		if i == 0 || i+1 >= len(tag) || tag[i] != ':' || tag[i+1] != '"' {
			break
		}
		name := tag[:i]
		tag = tag[i+1:]
		i = 1
		for i < len(tag) && tag[i] != '"' {
			if tag[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(tag) {
			break
		}
		val := tag[1:i]
		tag = tag[i+1:]
		if name == key {
			return val, true
		}
	}
	return "", false
}

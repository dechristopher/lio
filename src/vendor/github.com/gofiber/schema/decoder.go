// Copyright 2012 The Gorilla Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package schema

import (
	"encoding"
	"errors"
	"fmt"
	"maps"
	"mime/multipart"
	"reflect"
	"strings"
	"sync"
)

const (
	defaultMaxSize = 16000
)

// errNotPointerToStruct is returned by Decode for invalid destinations;
// hoisted so the check does not allocate on every call.
var errNotPointerToStruct = errors.New("schema: interface must be a pointer to struct")

var decodeValueBufferPool = sync.Pool{
	New: func() any {
		buf := make([]reflect.Value, 0, 8)
		return &buf
	},
}

// NewDecoder returns a new Decoder.
func NewDecoder() *Decoder {
	return &Decoder{cache: newCache(), maxSize: defaultMaxSize}
}

// Decoder decodes values from a map[string][]string to a struct.
type Decoder struct {
	cache             *cache
	zeroEmpty         bool
	ignoreUnknownKeys bool
	maxSize           int
}

// SetAliasTag changes the tag used to locate custom field aliases.
// The default tag is "schema".
func (d *Decoder) SetAliasTag(tag string) {
	d.cache.l.Lock()
	d.cache.tag = tag
	d.cache.reset()
	d.cache.l.Unlock()
}

// ZeroEmpty controls the behaviour when the decoder encounters empty values
// in a map.
// If z is true and a key in the map has the empty string as a value
// then the corresponding struct field is set to the zero value.
// If z is false then empty strings are ignored.
//
// The default value is false, that is empty values do not change
// the value of the struct field.
func (d *Decoder) ZeroEmpty(z bool) {
	d.zeroEmpty = z
}

// IgnoreUnknownKeys controls the behaviour when the decoder encounters unknown
// keys in the map.
// If i is true and an unknown field is encountered, it is ignored. This is
// similar to how unknown keys are handled by encoding/json.
// If i is false then Decode will return an error. Note that any valid keys
// will still be decoded in to the target struct.
//
// To preserve backwards compatibility, the default value is false.
func (d *Decoder) IgnoreUnknownKeys(i bool) {
	d.ignoreUnknownKeys = i
}

// MaxSize limits the size of slices for URL nested arrays or object arrays.
// Choose MaxSize carefully; large values may create many zero-value slice elements.
// Example: "items.100000=apple" would create a slice with 100,000 empty strings.
func (d *Decoder) MaxSize(size int) {
	d.maxSize = size
}

// RegisterConverter registers a converter function for a custom type.
func (d *Decoder) RegisterConverter(value interface{}, converterFunc Converter) {
	d.cache.registerConverter(value, converterFunc)
}

// Decode decodes a map[string][]string to a struct.
//
// The first parameter must be a pointer to a struct.
//
// The second parameter is a map, typically url.Values from an HTTP request.
// Keys are "paths" in dotted notation to the struct fields and nested structs.
//
// See the package documentation for a full explanation of the mechanics.
func (d *Decoder) Decode(dst interface{}, src map[string][]string, files ...map[string][]*multipart.FileHeader) (err error) {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return errNotPointerToStruct
	}

	// Catch panics from the decoder and return them as an error.
	// This is needed because the decoder calls reflect and reflect panics.
	// Installed before any other work so nothing can crash the caller.
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("schema: panic while decoding: %v", r)
			}
		}
	}()

	var multipartFiles map[string][]*multipart.FileHeader

	if len(files) > 0 {
		multipartFiles = files[0]
	}

	// Add files as empty string values to the decode view so path parsing
	// works uniformly. Work on a copy: the caller's src map must not be
	// mutated (and a caller-provided value under a file's key must not be
	// overwritten in it).
	if len(multipartFiles) > 0 {
		merged := make(map[string][]string, len(src)+len(multipartFiles))
		maps.Copy(merged, src)
		for path := range multipartFiles {
			merged[path] = []string{""}
		}
		src = merged
	}

	v = v.Elem()
	t := v.Type()
	rootInfo := d.cache.get(t)
	var multiErrors MultiError
	for path, values := range src {
		if parts, err := d.cache.parsePathInfo(path, rootInfo); err == nil {
			var filesSlice []*multipart.FileHeader
			if multipartFiles != nil {
				filesSlice = multipartFiles[path]
			}
			if err = d.decode(v, path, parts, values, filesSlice); err != nil {
				multiErrors = appendError(multiErrors, path, err)
			}
		} else {
			if errors.Is(err, errIndexTooLarge) {
				multiErrors = appendError(multiErrors, path, err)
			} else if !d.ignoreUnknownKeys {
				multiErrors = appendError(multiErrors, path, UnknownKeyError{Key: path})
			}
		}
	}
	if rootInfo.needsDefaultsWalk {
		multiErrors = mergeErrors(multiErrors, d.setDefaults(t, v, src, ""))
	}
	multiErrors = mergeErrors(multiErrors, d.checkRequired(rootInfo, src))
	if len(multiErrors) > 0 {
		return multiErrors
	}
	return nil
}

// setDefaults sets the default values when the `default` tag is specified,
// default is supported on basic/primitive types and their pointers,
// nested structs can also have default tags
func (d *Decoder) setDefaults(t reflect.Type, v reflect.Value, src map[string][]string, prefix string) MultiError {
	struc := d.cache.get(t)
	// Skip the walk entirely when it can have no effect (no default tags and
	// no anonymous embedded pointers to allocate anywhere in the tree) — the
	// overwhelmingly common case.
	if !struc.needsDefaultsWalk {
		return nil
	}

	var errs MultiError

	// Allocate nil anonymous embedded pointer fields so their promoted
	// fields stay reachable.
	for _, idx := range struc.anonymousPtrFields {
		if field := v.Field(idx); field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
	}

	for _, f := range struc.fields {
		vCurrent := walkIndexChain(v, f.index)
		if !vCurrent.IsValid() {
			// Unreachable behind an unsettable nil embedded pointer.
			continue
		}

		if vCurrent.Type().Kind() == reflect.Struct && f.defaultValue == "" {
			errs = mergeErrors(errs, d.setDefaults(vCurrent.Type(), vCurrent, src, prefix+f.canonicalAlias+"."))
		} else if isPointerToStruct(vCurrent) && f.defaultValue == "" {
			errs = mergeErrors(errs, d.setDefaults(vCurrent.Elem().Type(), vCurrent.Elem(), src, prefix+f.canonicalAlias+"."))
		}

		if f.defaultValue != "" && f.isRequired {
			errs = appendError(errs, "default-"+f.name, errors.New("required fields cannot have a default value"))
		} else if f.defaultValue != "" && vCurrent.IsZero() && !f.isRequired && !fieldProvided(src, prefix, f) {
			if f.typ.Kind() == reflect.Struct {
				errs = appendError(errs, "default-"+f.name, errors.New("default option is supported only on: bool, float variants, string, unit variants types or their corresponding pointers or slices"))
			} else if f.typ.Kind() == reflect.Slice {
				// check if slice has one of the supported types for defaults
				conv := getBuiltinConverter(f.typ.Elem().Kind())
				if conv == nil {
					errs = appendError(errs, "default-"+f.name, errors.New("default option is supported only on: bool, float variants, string, unit variants types or their corresponding pointers or slices"))
					continue
				}

				elemT := f.typ.Elem()
				defaultSlice := reflect.MakeSlice(f.typ, 0, strings.Count(f.defaultValue, "|")+1)
				for val := range strings.SplitSeq(f.defaultValue, "|") {
					// this check is to handle if the wrong value is provided
					convertedVal := conv(val)
					if !convertedVal.IsValid() {
						errs = appendError(errs, "default-"+f.name, fmt.Errorf("failed setting default: %s is not compatible with field %s type", val, f.name))
						break
					}
					// Builtin converters return the underlying kind; convert to
					// the (possibly named) element type before appending, else
					// reflect.Append panics for e.g. []MyInt.
					defaultSlice = reflect.Append(defaultSlice, convertedVal.Convert(elemT))
				}
				vCurrent.Set(defaultSlice)
			} else if f.typ.Kind() == reflect.Ptr {
				t1 := f.typ.Elem()

				if t1.Kind() == reflect.Struct || t1.Kind() == reflect.Slice {
					errs = appendError(errs, "default-"+f.name, errors.New("default option is supported only on: bool, float variants, string, unit variants types or their corresponding pointers or slices"))
				}

				// this check is to handle if the wrong value is provided
				if conv := getBuiltinConverter(t1.Kind()); conv != nil {
					if convertedVal := conv(f.defaultValue); convertedVal.IsValid() {
						// Build a pointer of the field's actual element type:
						// the converter yields the underlying kind, which is
						// convertible to the (possibly named) element type,
						// and *elem is assignable to the field even when the
						// field's type is itself a named pointer type (e.g.
						// type MyIntPtr *MyInt), where converting a *int
						// directly would panic.
						p := reflect.New(t1)
						p.Elem().Set(convertedVal.Convert(t1))
						vCurrent.Set(p)
					}
				}
			} else {
				// this check is to handle if the wrong value is provided
				conv := getBuiltinConverter(f.typ.Kind())
				if conv == nil {
					errs = appendError(errs, "default-"+f.name, errors.New("default option is supported only on: bool, float variants, string, unit variants types or their corresponding pointers or slices"))
				} else if convertedVal := conv(f.defaultValue); convertedVal.IsValid() {
					// Builtin converters return the underlying kind; convert to
					// the field's (possibly named) type before assigning.
					vCurrent.Set(convertedVal.Convert(f.typ))
				}
			}
		}
	}

	return errs
}

func isPointerToStruct(v reflect.Value) bool {
	return !v.IsZero() && v.Type().Kind() == reflect.Ptr && v.Elem().Type().Kind() == reflect.Struct
}

func fieldProvided(src map[string][]string, prefix string, f *fieldInfo) bool {
	for _, p := range f.paths(prefix) {
		if _, ok := src[p]; ok {
			return true
		}
	}
	return false
}

// checkRequired checks whether required fields are empty
//
// The set of required fields (including those of nested structs) is
// precomputed once per struct type in structInfo.requiredFields, so this
// only performs the per-request emptiness checks against src.
//
// src is the source map for decoding, we use it here to see if those required fields are included in src
func (d *Decoder) checkRequired(info *structInfo, src map[string][]string) MultiError {
	var errs MultiError
	for key, fields := range info.requiredFields {
		if isEmptyFields(fields, src) {
			errs = appendError(errs, key, EmptyFieldError{Key: key})
		}
	}
	return errs
}

type fieldWithPrefix struct {
	*fieldInfo
	prefix string
	// searchPaths lists the src keys this required field answers to, and
	// searchPathDots the corresponding nested-key prefixes; both are
	// precomputed at cache-build time so per-request checks allocate
	// nothing.
	searchPaths    []string
	searchPathDots []string
}

func newFieldWithPrefix(f *fieldInfo, prefix string) fieldWithPrefix {
	paths := f.paths(prefix)
	dots := make([]string, len(paths))
	for i, p := range paths {
		dots[i] = p + "."
	}
	return fieldWithPrefix{
		fieldInfo:      f,
		prefix:         prefix,
		searchPaths:    paths,
		searchPathDots: dots,
	}
}

// isEmptyFields returns true if all of specified fields are empty.
func isEmptyFields(fields []fieldWithPrefix, src map[string][]string) bool {
	for _, f := range fields {
		for i, path := range f.searchPaths {
			v, ok := src[path]
			if ok && !isEmpty(f.typ, v) {
				return false
			}
			// Check for nested keys that match this field.
			pathDot := f.searchPathDots[i]
			for key, val := range src {
				if len(val) == 0 {
					continue
				}
				// for nested structs
				if strings.HasPrefix(key, pathDot) {
					if !isEmpty(f.typ, val) {
						return false
					}
				}
			}
		}
	}
	return true
}

// isEmpty returns true if value is empty for specific type
func isEmpty(t reflect.Type, value []string) bool {
	if len(value) == 0 {
		return true
	}
	switch t.Kind() {
	case boolType, float32Type, float64Type,
		intType, int8Type, int16Type, int32Type, int64Type,
		stringType,
		uintType, uint8Type, uint16Type, uint32Type, uint64Type:
		return len(value[0]) == 0
	}
	return false
}

var (
	multipartFileHeaderPointerType      = reflect.TypeOf(&multipart.FileHeader{})
	sliceMultipartFileHeaderPointerType = reflect.TypeOf([]*multipart.FileHeader{})
)

// Supported multiple types:
// *multipart.FileHeader, *[]multipart.FileHeader, []*multipart.FileHeader
func handleMultipartField(field reflect.Value, files []*multipart.FileHeader) bool {
	fieldType := field.Type()
	if !isMultipartField(fieldType) {
		return false
	}

	// Skip if files are empty and field is multipart
	if len(files) == 0 {
		return true
	}

	// Check for *multipart.FileHeader
	if fieldType == multipartFileHeaderPointerType {
		field.Set(reflect.ValueOf(files[0]))
		return true
	}

	// Check for []*multipart.FileHeader
	if fieldType == sliceMultipartFileHeaderPointerType {
		field.Set(reflect.ValueOf(files))
		return true
	}

	// Check for *[]*multipart.FileHeader
	if fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()

		if field.IsNil() {
			field.Set(reflect.New(fieldType))
		}

		if fieldType == sliceMultipartFileHeaderPointerType {
			field.Elem().Set(reflect.ValueOf(files))
			return true
		}
	}

	return false
}

// Supported multiple types:
// *multipart.FileHeader, *[]multipart.FileHeader, []*multipart.FileHeader
func isMultipartField(typ reflect.Type) bool {
	// Check for *multipart.FileHeader
	if typ == multipartFileHeaderPointerType {
		return true
	}

	// Check for []*multipart.FileHeader
	if typ == sliceMultipartFileHeaderPointerType {
		return true
	}

	// Check for *[]*multipart.FileHeader
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()

		if typ == sliceMultipartFileHeaderPointerType {
			return true
		}
	}

	return false
}

// walkIndexChain walks v along a struct field index chain. Chains longer
// than one element traverse embedded structs; intermediate nil pointers are
// allocated so promoted fields stay reachable. It returns the zero Value
// when the chain is blocked by a nil pointer that cannot be set (an
// unexported embedded pointer), which callers treat as an unreachable
// field.
func walkIndexChain(v reflect.Value, chain []int) reflect.Value {
	for j, fi := range chain {
		if j > 0 && v.Kind() == reflect.Ptr {
			if v.IsNil() {
				if !v.CanSet() {
					return reflect.Value{}
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(fi)
	}
	return v
}

// decode fills a struct field using a parsed path.
func (d *Decoder) decode(v reflect.Value, path string, parts []pathPart, values []string, files []*multipart.FileHeader) error {
	// Get the field walking the struct fields by index.
	for _, hop := range parts[0].hops {
		// A previous hop may have been blocked by an unsettable nil
		// embedded pointer; the field is unreachable then.
		if !v.IsValid() {
			return nil
		}
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}

		// Allocate embedded anonymous pointers required for promoted fields.
		for _, idx := range hop.ensure {
			if f := v.Field(idx); f.IsNil() {
				f.Set(reflect.New(f.Type().Elem()))
			}
		}

		v = walkIndexChain(v, hop.index)
	}

	// Don't even bother for unexported fields.
	if !v.CanSet() {
		return nil
	}

	// Check multipart files
	if parts[0].field.isMultipart && handleMultipartField(v, files) {
		return nil
	}

	// Dereference if needed.
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		if v.IsNil() {
			v.Set(reflect.New(t))
		}
		v = v.Elem()
	}

	// Slice of structs. Let's go recursive.
	if len(parts) > 1 {
		idx := parts[0].index
		// a defensive check to avoid creating a large slice based on user input index
		if idx > d.maxSize {
			return fmt.Errorf("%v index %d is larger than the configured maxSize %d", v.Kind(), idx, d.maxSize)
		}
		if v.IsNil() || v.Len() < idx+1 {
			// Grow into a fresh backing array: extending within existing
			// capacity would write into memory the caller may still share
			// through other slices aliasing the original array.
			value := reflect.MakeSlice(t, idx+1, idx+1)
			if v.Len() > 0 {
				// Resize it.
				reflect.Copy(value, v)
			}
			v.Set(value)
		}
		return d.decode(v.Index(idx), path, parts[1:], values, files)
	}

	// Get the converter early in case there is one for a slice type.
	conv := d.cache.converter(t)
	// The encoding.TextUnmarshaler facts for v's type are precomputed per
	// field; instances are bound to live values where needed below. For an
	// elem part (path terminated at a slice index) v is an element of the
	// slice field, so the element type's facts apply.
	m := parts[0].field.derefUnmarshaler
	if parts[0].elem {
		m = parts[0].field.elemUnmarshaler
	}
	if conv == nil && t.Kind() == reflect.Slice && m.IsSliceElement {
		elemT := t.Elem()
		isPtrElem := elemT.Kind() == reflect.Ptr
		if isPtrElem {
			elemT = elemT.Elem()
		}

		// Try to get a converter for the element type.
		customConv := d.cache.converter(elemT)
		conv := customConv
		if conv == nil {
			conv = getBuiltinConverter(elemT.Kind())
			if conv == nil {
				// As we are not dealing with slice of structs here, we don't need to check if the type
				// implements TextUnmarshaler interface
				return fmt.Errorf("schema: converter not found for %v", elemT)
			}
		}

		// Fast path: builtin element kinds without unmarshalers, custom
		// converters or pointer elements decode straight into a fresh slice,
		// avoiding one reflect.Value allocation per element.
		if customConv == nil && !m.IsValid && !isPtrElem {
			return d.decodeBuiltinSlice(v, t, path, values)
		}

		itemsBuf := decodeValueBufferPool.Get().(*[]reflect.Value)
		items := (*itemsBuf)[:0]
		defer func() {
			clear(items)
			*itemsBuf = items[:0]
			decodeValueBufferPool.Put(itemsBuf)
		}()

		for key, value := range values {
			if value == "" {
				if d.zeroEmpty {
					items = append(items, reflect.Zero(t.Elem()))
				}
			} else if m.IsValid {
				u := reflect.New(elemT)
				if m.IsSliceElementPtr {
					u = reflect.New(reflect.PointerTo(elemT).Elem())
				}
				um, _ := reflect.TypeAssert[encoding.TextUnmarshaler](u)
				if err := um.UnmarshalText([]byte(value)); err != nil {
					return ConversionError{
						Key:   path,
						Type:  t,
						Index: key,
						Err:   err,
					}
				}
				if m.IsSliceElementPtr {
					items = append(items, u.Elem().Addr())
				} else {
					// u is always a pointer from reflect.New; store the
					// pointed-to value.
					items = append(items, u.Elem())
				}
			} else if item := conv(value); item.IsValid() {
				items = appendConvertedItem(items, item, elemT, isPtrElem)
			} else {
				if strings.IndexByte(value, ',') != -1 {
					for value := range strings.SplitSeq(value, ",") {
						if value == "" {
							if d.zeroEmpty {
								items = append(items, reflect.Zero(t.Elem()))
							}
						} else if item := conv(value); item.IsValid() {
							items = appendConvertedItem(items, item, elemT, isPtrElem)
						} else {
							return ConversionError{
								Key:   path,
								Type:  elemT,
								Index: key,
							}
						}
					}
				} else {
					return ConversionError{
						Key:   path,
						Type:  elemT,
						Index: key,
					}
				}
			}
		}
		value := reflect.MakeSlice(t, len(items), len(items))
		for i, item := range items {
			value.Index(i).Set(item)
		}
		v.Set(value)
	} else {
		val := ""
		// Use the last value provided if any values were provided
		if len(values) > 0 {
			val = values[len(values)-1]
		}

		if conv != nil {
			if value := conv(val); value.IsValid() {
				v.Set(value.Convert(t))
			} else {
				return ConversionError{
					Key:   path,
					Type:  t,
					Index: -1,
				}
			}
		} else if m.IsValid {
			if m.IsPtr {
				u := reflect.New(v.Type())
				um, _ := reflect.TypeAssert[encoding.TextUnmarshaler](u)
				if err := um.UnmarshalText([]byte(val)); err != nil {
					return ConversionError{
						Key:   path,
						Type:  t,
						Index: -1,
						Err:   err,
					}
				}
				v.Set(reflect.Indirect(u))
			} else {
				// If the value implements the encoding.TextUnmarshaler interface
				// apply UnmarshalText as the converter, binding it to the
				// live value.
				um, _ := reflect.TypeAssert[encoding.TextUnmarshaler](v)
				if err := um.UnmarshalText([]byte(val)); err != nil {
					return ConversionError{
						Key:   path,
						Type:  t,
						Index: -1,
						Err:   err,
					}
				}
			}
		} else if val == "" {
			if d.zeroEmpty {
				v.Set(reflect.Zero(t))
			}
		} else if handled, ok := setBuiltinKind(v, t.Kind(), val); handled {
			if !ok {
				return ConversionError{
					Key:   path,
					Type:  t,
					Index: -1,
				}
			}
		} else {
			return fmt.Errorf("schema: converter not found for %v", t)
		}
	}
	return nil
}

// appendConvertedItem converts a builtin/custom converter result to the slice
// element type and appends it, wrapping it in a freshly allocated pointer for
// pointer-element slices. The conversion must happen before the pointer wrap:
// builtin converters return the underlying kind (e.g. int for a named
// `type MyInt int`), which is not assignable to the named element type, so
// Set-ing it into a *MyInt without converting first panics.
func appendConvertedItem(items []reflect.Value, item reflect.Value, elemT reflect.Type, isPtrElem bool) []reflect.Value {
	if item.Type() != elemT {
		item = item.Convert(elemT)
	}
	if isPtrElem {
		ptr := reflect.New(elemT)
		ptr.Elem().Set(item)
		item = ptr
	}
	return append(items, item)
}

// decodeBuiltinSlice decodes values into the slice field v of type t whose
// elements are builtin-convertible kinds, parsing directly into slice slots
// instead of boxing every element in a reflect.Value. The slice is built
// detached and only assigned to v when every value parsed, matching the
// all-or-nothing behavior of the generic path.
//
// A value that fails to parse as a whole is retried as a comma-separated
// list. For non-string kinds a value containing a comma can never parse as a
// whole (no builtin syntax admits commas — pinned by a test), and string
// values always parse, so item boundaries are knowable upfront: the slice is
// sized by a cheap comma count (an upper bound, since empty items may be
// skipped) and truncated to the filled length at the end.
func (d *Decoder) decodeBuiltinSlice(v reflect.Value, t reflect.Type, path string, values []string) error {
	elemT := t.Elem()
	k := elemT.Kind()
	split := k != reflect.String

	n := 0
	for _, value := range values {
		if split {
			n += strings.Count(value, ",")
		}
		n++
	}

	sl := reflect.MakeSlice(t, n, n)
	i := 0
	for key, value := range values {
		switch {
		case value == "":
			if d.zeroEmpty {
				i++ // slot stays zero
			}
		case split && strings.IndexByte(value, ',') != -1:
			for item := range strings.SplitSeq(value, ",") {
				if item == "" {
					if d.zeroEmpty {
						i++ // slot stays zero
					}
					continue
				}
				if _, ok := setBuiltinKind(sl.Index(i), k, item); !ok {
					return ConversionError{
						Key:   path,
						Type:  elemT,
						Index: key,
					}
				}
				i++
			}
		default:
			if _, ok := setBuiltinKind(sl.Index(i), k, value); !ok {
				return ConversionError{
					Key:   path,
					Type:  elemT,
					Index: key,
				}
			}
			i++
		}
	}
	if i < n {
		sl = sl.Slice(0, i)
	}
	v.Set(sl)
	return nil
}

func isTextUnmarshaler(v reflect.Value) unmarshaler {
	// Create a new unmarshaller instance
	m := unmarshaler{}
	if _, m.IsValid = reflect.TypeAssert[encoding.TextUnmarshaler](v); m.IsValid {
		return m
	}
	// As the UnmarshalText function should be applied to the pointer of the
	// type, we check that type to see if it implements the necessary
	// method.
	if _, m.IsValid = reflect.TypeAssert[encoding.TextUnmarshaler](reflect.New(v.Type())); m.IsValid {
		m.IsPtr = true
		return m
	}

	// if v is []T or *[]T create new T
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Slice {
		// The slice type itself cannot implement encoding.TextUnmarshaler
		// here: the value-level assert above already covered it. Check
		// whether the elements do.
		m.IsSliceElement = true
		if t = t.Elem(); t.Kind() == reflect.Ptr {
			t = reflect.PointerTo(t.Elem())
			m.IsSliceElementPtr = true
			_, m.IsValid = reflect.TypeAssert[encoding.TextUnmarshaler](reflect.Zero(t))
			return m
		}
	}

	_, m.IsValid = reflect.TypeAssert[encoding.TextUnmarshaler](reflect.New(t))
	return m
}

// TextUnmarshaler helpers ----------------------------------------------------
// unmarshaler describes how a type relates to encoding.TextUnmarshaler.
// It carries type-level facts only; the decoder binds instances to live
// values at the point of use.
type unmarshaler struct {
	// IsValid indicates whether the resolved type indicated by the other
	// flags implements the encoding.TextUnmarshaler interface.
	IsValid bool
	// IsPtr indicates that the resolved type is the pointer of the original
	// type.
	IsPtr bool
	// IsSliceElement indicates that the resolved type is a slice element of
	// the original type.
	IsSliceElement bool
	// IsSliceElementPtr indicates that the resolved type is a pointer to a
	// slice element of the original type.
	IsSliceElementPtr bool
}

// Errors ---------------------------------------------------------------------

// ConversionError stores information about a failed conversion.
type ConversionError struct {
	Key   string       // key from the source map.
	Type  reflect.Type // expected type of elem
	Index int          // index for multi-value fields; -1 for single-value fields.
	Err   error        // low-level error (when it exists)
}

func (e ConversionError) Error() string {
	var output string

	if e.Index < 0 {
		output = fmt.Sprintf("schema: error converting value for %q", e.Key)
	} else {
		output = fmt.Sprintf("schema: error converting value for index %d of %q",
			e.Index, e.Key)
	}

	if e.Err != nil {
		output = fmt.Sprintf("%s. Details: %s", output, e.Err)
	}

	return output
}

// UnknownKeyError stores information about an unknown key in the source map.
type UnknownKeyError struct {
	Key string // key from the source map.
}

func (e UnknownKeyError) Error() string {
	return fmt.Sprintf("schema: invalid path %q", e.Key)
}

// EmptyFieldError stores information about an empty required field.
type EmptyFieldError struct {
	Key string // required key in the source map.
}

func (e EmptyFieldError) Error() string {
	return fmt.Sprintf("%v is empty", e.Key)
}

// MultiError stores multiple decoding errors.
//
// Borrowed from the App Engine SDK.
type MultiError map[string]error

func (e MultiError) Error() string {
	s := ""
	for _, err := range e {
		s = err.Error()
		break
	}
	switch len(e) {
	case 0:
		return "(0 errors)"
	case 1:
		return s
	case 2:
		return s + " (and 1 other error)"
	}
	return fmt.Sprintf("%s (and %d other errors)", s, len(e)-1)
}

func appendRequiredField(m map[string][]fieldWithPrefix, key string, field fieldWithPrefix) map[string][]fieldWithPrefix {
	if m == nil {
		m = make(map[string][]fieldWithPrefix)
	}
	m[key] = append(m[key], field)
	return m
}

func appendError(m MultiError, key string, err error) MultiError {
	if err == nil {
		return m
	}
	if m == nil {
		m = make(MultiError)
	}
	if m[key] == nil {
		m[key] = err
	}
	return m
}

func mergeErrors(dst, src MultiError) MultiError {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(MultiError, len(src))
	}
	for key, err := range src {
		if dst[key] == nil {
			dst[key] = err
		}
	}
	return dst
}

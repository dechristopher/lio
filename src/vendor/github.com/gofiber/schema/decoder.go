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
	"strconv"
	"strings"
	"sync"
	"unsafe"

	utils "github.com/gofiber/utils/v2"
)

const (
	defaultMaxSize = 16000
)

// errNotPointerToStruct is returned by Decode for invalid destinations;
// hoisted so the check does not allocate on every call.
var errNotPointerToStruct = errors.New("schema: interface must be a pointer to struct")

// fileKeyValues stands in for the value of a multipart file's key in the
// decode view. Nothing writes through a source map's values and the map it
// goes into is this package's own copy, so one slice serves every file key.
var fileKeyValues = []string{""}

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
func (d *Decoder) Decode(dst interface{}, src map[string][]string, files ...map[string][]*multipart.FileHeader) error {
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
			merged[path] = fileKeyValues
		}
		src = merged
	}

	s := source{m: src}
	return d.decodeSource(dst, &s, multipartFiles)
}

// DecodeValues decodes key/value pairs to a struct as Decode decodes the map
// they make: values[i] is a value of the key keys[i], and a key given more
// than once has all of its values, in the order given. It saves a caller that
// has the pairs one at a time, as a parsed query string does, building that
// map, and saves the decode iterating it.
//
// Where several keys name the same field, as keys that differ only in case
// do, the keys decode in the order of their first pairs, so the key given
// first the latest wins; Decode lets whichever its map yields last win.
//
// The first parameter must be a pointer to a struct, and keys and values must
// be the same length. The slices are only read, and only during the call.
func (d *Decoder) DecodeValues(dst interface{}, keys, values []string) error {
	if len(keys) != len(values) {
		return errValuesLength
	}
	// The index is set up field by field where it lies. Go puts a literal for
	// a variable whose address is taken, or a call's result, together
	// elsewhere and copies it over, and the copy reads back in wide loads the
	// words just stored in narrow ones, which stalls each load.
	var p pairIndex
	p.keys, p.values = keys, values
	if n := len(keys); n <= maxScanPairs {
		var next, last [maxScanPairs]int32
		p.next, p.last = next[:n], last[:n]
	} else if n <= maxInlinePairs {
		var table [2 * maxInlinePairs]int32
		var next, last [maxInlinePairs]int32
		p.table, p.next, p.last = table[:indexSize(n)], next[:n], last[:n]
	} else {
		p.table, p.next, p.last = make([]int32, indexSize(n)), make([]int32, n), make([]int32, n)
	}
	p.index()
	s := source{pairs: &p}
	return d.decodeSource(dst, &s, nil)
}

// decodeSource decodes src into dst, the pointer to a struct Decode and
// DecodeValues were given, with the files Decode was.
func (d *Decoder) decodeSource(dst interface{}, src *source, multipartFiles map[string][]*multipart.FileHeader) (err error) {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
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

	v = v.Elem()
	t := v.Type()
	rootInfo := d.cache.get(t)
	// Required-key bookkeeping rides along with the loop below so src is
	// walked once: the direct lookups settle almost every group here, and
	// the rest are answered by the nested keys the loop visits anyway.
	var satisfied []uint64
	pending := 0
	if len(rootInfo.requiredGroups) > 0 {
		// Declared here so a struct with no required keys never pays to
		// zero it.
		var requiredBits [requiredBitWords]uint64
		satisfied, pending = markProvidedDirectly(rootInfo.requiredGroups, src, requiredBits[:])
	}
	// Only a struct whose paths carry an index can grow anything.
	var grow *growTracker
	if rootInfo.hasIndexedSlice {
		var tracker growTracker
		grow = &tracker
	}
	// The state decodeKey carries from key to key is kept in variables of
	// its own rather than a struct: the compiler would let the stack values
	// above escape along with the errors a struct holding both returns.
	var errs MultiError
	if p := src.pairs; p != nil {
		// A key whose pairs are not adjacent gathers its values here.
		var scratch [8]string
		for i, path := range p.keys {
			if p.first(i) {
				errs, pending = d.decodeKey(v, rootInfo, multipartFiles, grow, satisfied, path, p.group(i, scratch[:0]), errs, pending)
			}
		}
	} else {
		for path, values := range src.m {
			errs, pending = d.decodeKey(v, rootInfo, multipartFiles, grow, satisfied, path, values, errs, pending)
		}
	}
	if rootInfo.needsDefaultsWalk {
		errs = mergeErrors(errs, d.setDefaults(t, v, src, ""))
	}
	errs = mergeErrors(errs, missingRequired(rootInfo.requiredGroups, satisfied, pending))
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// decodeKey decodes the values of one source key into v, the struct whose
// metadata is rootInfo, and returns errs with any error it met added, and
// pending less the required groups the key settles in satisfied.
func (d *Decoder) decodeKey(
	v reflect.Value, rootInfo *structInfo, files map[string][]*multipart.FileHeader, grow *growTracker,
	satisfied []uint64, path string, values []string, errs MultiError, pending int,
) (MultiError, int) {
	if pending > 0 {
		if i := strings.IndexByte(path, '.'); i >= 0 && len(values) > 0 {
			pending = markProvidedByNestedKey(rootInfo, path, i+1, values, satisfied, pending)
		}
	}
	if parts, err := d.cache.parsePathInfo(path, rootInfo); err == nil {
		var filesSlice []*multipart.FileHeader
		if files != nil {
			filesSlice = files[path]
		}
		if err = d.decode(v, path, parts, values, filesSlice, grow); err != nil {
			errs = appendError(errs, path, err)
		}
		// A key that names no field is by far the most common failure,
		// and parsePathInfo returns that sentinel unwrapped, so the
		// pointer compare keeps errors.Is off the per-key path.
	} else if err != errInvalidPath && errors.Is(err, errIndexTooLarge) { //nolint:errorlint // see above
		errs = appendError(errs, path, err)
	} else if !d.ignoreUnknownKeys {
		errs = appendError(errs, path, UnknownKeyError{Key: path})
	}
	return errs, pending
}

// setDefaults sets the default values when the `default` tag is specified,
// default is supported on basic/primitive types and their pointers,
// nested structs can also have default tags
var (
	errUnsupportedDefault = errors.New("default option is supported only on: bool, float variants, string, unit variants types or their corresponding pointers or slices")
	errRequiredDefault    = errors.New("required fields cannot have a default value")
)

// resolveDefault converts the field's default option once, when the struct
// metadata is built, so setDefaults assigns a resolved value on every call
// instead of converting the string each time. Slice and pointer defaults are
// still instantiated per call, so no two decoded values share one.
func (f *fieldInfo) resolveDefault() {
	if f.defaultValue == "" {
		return
	}
	def := &fieldDefault{}
	f.def = def
	switch f.typ.Kind() {
	case reflect.Slice:
		elemT := f.typ.Elem()
		conv := getBuiltinConverter(elemT.Kind())
		if conv == nil {
			return
		}
		tmpl := reflect.MakeSlice(f.typ, 0, strings.Count(f.defaultValue, "|")+1)
		for val := range strings.SplitSeq(f.defaultValue, "|") {
			v := conv(val)
			if !v.IsValid() {
				def.err = fmt.Errorf("failed setting default: %s is not compatible with field %s type", val, f.name)
				break
			}
			// Builtin converters return the underlying kind; convert to the
			// (possibly named) element type, else Append panics for []MyInt.
			tmpl = reflect.Append(tmpl, v.Convert(elemT))
		}
		def.slice = tmpl
	case reflect.Ptr:
		t1 := f.typ.Elem()
		if conv := getBuiltinConverter(t1.Kind()); conv != nil {
			if v := conv(f.defaultValue); v.IsValid() {
				def.val = v.Convert(t1)
			}
		}
	default:
		if conv := getBuiltinConverter(f.typ.Kind()); conv != nil {
			if v := conv(f.defaultValue); v.IsValid() {
				def.val = v.Convert(f.typ)
			}
		}
	}
}

func (d *Decoder) setDefaults(t reflect.Type, v reflect.Value, src *source, prefix string) MultiError {
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

	for _, f := range struc.defaultFields {
		vCurrent := walkIndexChain(v, f.index)
		if !vCurrent.IsValid() {
			// Unreachable behind an unsettable nil embedded pointer.
			continue
		}

		// vCurrent's type is f.typ; a nested struct is walked under its own
		// prefix (an empty prefix concatenates without allocating).
		kind := f.typ.Kind()
		if kind == reflect.Struct && f.defaultValue == "" {
			errs = mergeErrors(errs, d.setDefaults(f.typ, vCurrent, src, prefix+f.canonicalDot))
		} else if kind == reflect.Ptr && f.defaultValue == "" && isPointerToStruct(vCurrent) {
			errs = mergeErrors(errs, d.setDefaults(f.typ.Elem(), vCurrent.Elem(), src, prefix+f.canonicalDot))
		}

		def := f.def
		if def == nil {
			continue
		}
		if f.isRequired {
			errs = appendError(errs, "default-"+f.name, errRequiredDefault)
			continue
		}
		if !vCurrent.IsZero() || fieldProvided(src, prefix, f) {
			continue
		}
		// The default itself was resolved when the metadata was built; see
		// resolveDefault. Only the per-call parts remain: the errors a kind
		// that takes no default reports, and a fresh pointer or slice so no
		// two decoded values share one.
		switch kind {
		case reflect.Struct:
			errs = appendError(errs, "default-"+f.name, errUnsupportedDefault)
		case reflect.Slice:
			if !def.slice.IsValid() {
				errs = appendError(errs, "default-"+f.name, errUnsupportedDefault)
				continue
			}
			if def.err != nil {
				errs = appendError(errs, "default-"+f.name, def.err)
			}
			fresh := reflect.MakeSlice(f.typ, def.slice.Len(), def.slice.Cap())
			reflect.Copy(fresh, def.slice)
			vCurrent.Set(fresh)
		case reflect.Ptr:
			t1 := f.typ.Elem()
			if t1.Kind() == reflect.Struct || t1.Kind() == reflect.Slice {
				errs = appendError(errs, "default-"+f.name, errUnsupportedDefault)
			}
			if def.val.IsValid() {
				// *elem is assignable to the field even when the field's
				// type is itself a named pointer type (type MyIntPtr *MyInt),
				// where converting a *int directly would panic.
				p := reflect.New(t1)
				p.Elem().Set(def.val)
				vCurrent.Set(p)
			}
		default:
			if getBuiltinConverter(kind) == nil {
				errs = appendError(errs, "default-"+f.name, errUnsupportedDefault)
			} else if def.val.IsValid() {
				vCurrent.Set(def.val)
			}
		}
	}

	return errs
}

// growCap is the capacity to give a slice being grown to n elements: room for
// another doubling when the slice is tracked, so later indices mostly land
// inside it, and exactly n when it is not, since the extra would go unused.
func (d *Decoder) growCap(n int, tracked bool) int {
	if !tracked {
		return n
	}
	c := 2 * n
	if c < n || c > d.maxSize+1 { // overflow, or past what maxSize admits
		c = d.maxSize + 1
	}
	if c < n {
		c = n
	}
	return c
}

func isPointerToStruct(v reflect.Value) bool {
	return !v.IsZero() && v.Type().Kind() == reflect.Ptr && v.Elem().Type().Kind() == reflect.Struct
}

func fieldProvided(src *source, prefix string, f *fieldInfo) bool {
	if keyProvided(src, prefix, f.alias) {
		return true
	}
	return f.alias != f.canonicalAlias && keyProvided(src, prefix, f.canonicalAlias)
}

// keyProvided reports whether prefix+name is a key of src, assembling the key
// in a stack buffer when it fits so the probe allocates nothing.
func keyProvided(src *source, prefix, name string) bool {
	if prefix == "" {
		_, ok := src.lookup(name)
		return ok
	}
	if n := len(prefix) + len(name); n <= maxDirectKeyLen {
		var buf [maxDirectKeyLen]byte
		copy(buf[copy(buf[:], prefix):], name)
		return src.hasBytes(buf[:n])
	}
	_, ok := src.lookup(prefix + name)
	return ok
}

// The set of required fields (including those of nested structs) is
// precomputed once per struct type in structInfo.requiredGroups, so what
// follows only performs the per-request emptiness checks against src.
//
// A group is satisfied by a value under one of its own paths, or by any
// nested key below one of them ("d.e" satisfies required "d"). Direct paths
// are looked up first, since they settle almost every group; what is left is
// answered by walking the dotted prefixes of each source key. Decode runs
// those three steps around its own loop over src; checkRequired composes them
// for a standalone check.

// requiredBitWords sizes the inline satisfied-group bitset: 256 required
// keys, past which it is allocated.
const requiredBitWords = 4

// markProvidedDirectly marks every required group src answers through one of
// its own paths, returning the bitset — backed by inline when the groups fit
// — and how many are still pending.
func markProvidedDirectly(groups []requiredGroup, src *source, inline []uint64) ([]uint64, int) {
	if len(groups) == 0 {
		return nil, 0
	}
	var satisfied []uint64
	if w := (len(groups) + 63) >> 6; w <= len(inline) {
		satisfied = inline[:w]
	} else {
		satisfied = make([]uint64, w)
	}
	pending := len(groups)
	for gi := range groups {
		if directlyProvided(groups[gi].fields, src) {
			satisfied[gi>>6] |= 1 << uint(gi&63)
			pending--
		}
	}
	return satisfied, pending
}

// markProvidedByNestedKey walks the dotted prefixes of key from off, marking
// every required group they name that val is non-empty for, and returns the
// new pending count. Callers test for a dotted, non-empty key themselves, so
// the keys that can answer nothing do not pay for a call.
func markProvidedByNestedKey(info *structInfo, key string, off int, val []string, satisfied []uint64, pending int) int {
	for {
		for _, p := range info.requiredPrefixes[key[:off]] {
			if satisfied[p.group>>6]&(1<<uint(p.group&63)) != 0 {
				continue
			}
			if !isEmpty(p.typ, val) {
				satisfied[p.group>>6] |= 1 << uint(p.group&63)
				pending--
			}
		}
		i := strings.IndexByte(key[off:], '.')
		if i < 0 {
			return pending
		}
		off += i + 1
	}
}

// missingRequired reports the required keys nothing in src answered.
func missingRequired(groups []requiredGroup, satisfied []uint64, pending int) MultiError {
	if pending == 0 {
		return nil
	}
	var errs MultiError
	for gi := range groups {
		if satisfied[gi>>6]&(1<<uint(gi&63)) == 0 {
			errs = appendError(errs, groups[gi].key, EmptyFieldError{Key: groups[gi].key})
		}
	}
	return errs
}

// checkRequired reports which of info's required keys src leaves empty: the
// standalone form of what Decode folds into its own loop.
func (d *Decoder) checkRequired(info *structInfo, src map[string][]string) MultiError {
	var inline [requiredBitWords]uint64
	s := source{m: src}
	satisfied, pending := markProvidedDirectly(info.requiredGroups, &s, inline[:])
	if pending > 0 {
		for key, val := range src {
			i := strings.IndexByte(key, '.')
			if i < 0 || len(val) == 0 {
				continue
			}
			if pending = markProvidedByNestedKey(info, key, i+1, val, satisfied, pending); pending == 0 {
				break
			}
		}
	}
	return missingRequired(info.requiredGroups, satisfied, pending)
}

// requiredGroup is one required key together with the fields that can
// satisfy it; its position in structInfo.requiredGroups is its bitset index.
type requiredGroup struct {
	key    string
	fields []fieldWithPrefix
}

// requiredPrefix names a group a nested source key can satisfy, with the
// field type that judges whether the key's value counts as non-empty.
type requiredPrefix struct {
	typ   reflect.Type
	group int
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

// directlyProvided reports whether any of the group's own paths carries a
// non-empty value in src.
func directlyProvided(fields []fieldWithPrefix, src *source) bool {
	for _, f := range fields {
		for _, path := range f.searchPaths {
			if v, ok := src.lookup(path); ok && !isEmpty(f.typ, v) {
				return true
			}
		}
	}
	return false
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

// walkHops follows a path to the target field, dereferencing pointers and
// allocating the embedded pointers promoted fields need on the way. It
// returns the zero Value when an unsettable nil embedded pointer blocks the
// walk.
func walkHops(v reflect.Value, hops []pathHop) reflect.Value {
	for _, hop := range hops {
		// A previous hop may have been blocked by an unsettable nil
		// embedded pointer; the field is unreachable then.
		if !v.IsValid() {
			return v
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
	return v
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

// growTracker remembers which of the destination struct's slice fields a
// Decode call has already reallocated, and how much room it reserved in each.
// Fields are keyed by their index in that struct, which is what pathPart's
// soleIndex carries: only a slice reached by one hop can be tracked, since a
// deeper path reaches a slice inside some element, which the field alone
// cannot tell apart from the same field in another. Fields past the four
// entries keep growing exactly.
type growTracker struct {
	fields [4]int
	caps   [4]int
	n      int
}

// reserved returns the capacity this call gave the slice at struct field
// index i, or 0 if it has not reallocated it.
func (g *growTracker) reserved(i int) int {
	for j := 0; j < g.n; j++ {
		if g.fields[j] == i {
			return g.caps[j]
		}
	}
	return 0
}

func (g *growTracker) record(i, c int) {
	for j := 0; j < g.n; j++ {
		if g.fields[j] == i {
			g.caps[j] = c
			return
		}
	}
	if g.n < len(g.fields) {
		g.fields[g.n] = i
		g.caps[g.n] = c
		g.n++
	}
}

// decode fills a struct field using a parsed path. grow is non-nil only for
// the outermost call, the one place slice growth can be tracked.
func (d *Decoder) decode(v reflect.Value, path string, parts []pathPart, values []string, files []*multipart.FileHeader, grow *growTracker) error {
	// Get the field walking the struct fields by index. Almost every path is
	// one hop into a field of v, which needs none of the loop's bookkeeping.
	if idx := parts[0].soleIndex; idx >= 0 && v.Kind() == reflect.Struct {
		v = v.Field(idx)
	} else if v = walkHops(v, parts[0].hops); !v.IsValid() {
		// Unreachable behind an unsettable nil embedded pointer.
		return nil
	}

	// Don't even bother for unexported fields.
	if !v.CanSet() {
		return nil
	}

	// Fast path: plain builtin scalar fields skip the converter/unmarshaler
	// dispatch below; fastKind was validated at cache-build time.
	if k := parts[0].field.fastKind; k != reflect.Invalid && len(parts) == 1 && !parts[0].elem {
		val := ""
		if len(values) > 0 {
			val = values[len(values)-1]
		}
		if val == "" {
			if d.zeroEmpty {
				v.SetZero()
			}
			return nil
		}
		if _, ok := setBuiltinKind(v, k, val); !ok {
			return ConversionError{
				Key:   path,
				Type:  parts[0].field.typ,
				Index: -1,
			}
		}
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
		if n := idx + 1; v.IsNil() || v.Len() < n {
			// Indices arrive in map order, so a slice is typically grown
			// several times per call. Extending one this call allocated is
			// free: the room past its length is ours and freshly zeroed.
			owner := -1
			if grow != nil {
				owner = parts[0].soleIndex
			}
			if owner >= 0 && n <= v.Cap() && grow.reserved(owner) >= n {
				v.SetLen(n)
			} else {
				// Otherwise grow into a fresh backing array: extending within
				// the existing capacity would write into memory the caller
				// may still share through another slice aliasing it.
				value := reflect.MakeSlice(t, n, d.growCap(n, owner >= 0))
				if v.Len() > 0 {
					// Resize it.
					reflect.Copy(value, v)
				}
				v.Set(value)
				if owner >= 0 {
					grow.record(owner, value.Cap())
				}
			}
		}
		return d.decode(v.Index(idx), path, parts[1:], values, files, nil)
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

		return d.decodeBoxedSlice(v, t, elemT, path, values, conv, m, isPtrElem)
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

// decodeBoxedSlice decodes values into the slice field v whose elements have
// to round-trip through a reflect.Value: a custom converter, a
// TextUnmarshaler, or pointer elements. It is split out so that decode, which
// runs once per source key, carries no defer of its own.
func (d *Decoder) decodeBoxedSlice(v reflect.Value, t, elemT reflect.Type, path string, values []string, conv Converter, m unmarshaler, isPtrElem bool) error {
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

	n := len(values)
	if split {
		for _, value := range values {
			n += strings.Count(value, ",")
		}
		// The counts already say whether any value holds a separator, so
		// when none does the per-value scans below have nothing to find.
		split = n > len(values)
	}

	// Exact builtin slice types decode without per-element reflect calls;
	// named slice or element types fall through to the generic path.
	switch t {
	case typSliceString:
		return decodeNativeSlice(d.zeroEmpty, v, path, values, elemT, n, split, parseNativeString)
	case typSliceInt:
		return decodeNativeSlice(d.zeroEmpty, v, path, values, elemT, n, split, parseNativeInt)
	case typSliceInt64:
		return decodeNativeSlice(d.zeroEmpty, v, path, values, elemT, n, split, parseNativeInt64)
	case typSliceUint:
		return decodeNativeSlice(d.zeroEmpty, v, path, values, elemT, n, split, parseNativeUint)
	case typSliceUint64:
		return decodeNativeSlice(d.zeroEmpty, v, path, values, elemT, n, split, parseNativeUint64)
	case typSliceFloat64:
		return decodeNativeSlice(d.zeroEmpty, v, path, values, elemT, n, split, parseNativeFloat64)
	case typSliceBool:
		return decodeNativeSlice(d.zeroEmpty, v, path, values, elemT, n, split, parseNativeBool)
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

// Exact (unnamed) builtin slice types eligible for the native decode path.
var (
	typSliceString  = reflect.TypeOf([]string(nil))
	typSliceInt     = reflect.TypeOf([]int(nil))
	typSliceInt64   = reflect.TypeOf([]int64(nil))
	typSliceUint    = reflect.TypeOf([]uint(nil))
	typSliceUint64  = reflect.TypeOf([]uint64(nil))
	typSliceFloat64 = reflect.TypeOf([]float64(nil))
	typSliceBool    = reflect.TypeOf([]bool(nil))
)

// decodeNativeSlice mirrors the generic decodeBuiltinSlice loop for a field
// typed exactly []T, parsing into a native slice assigned only if every value
// parsed (all-or-nothing). n is the precomputed element upper bound.
func decodeNativeSlice[T any](zeroEmpty bool, v reflect.Value, path string, values []string, elemT reflect.Type, n int, split bool, parse func(string) (T, bool)) error {
	out := make([]T, 0, n)
	var zero T
	for key, value := range values {
		switch {
		case value == "":
			if zeroEmpty {
				out = append(out, zero)
			}
		case split && strings.IndexByte(value, ',') != -1:
			for item := range strings.SplitSeq(value, ",") {
				if item == "" {
					if zeroEmpty {
						out = append(out, zero)
					}
					continue
				}
				ev, ok := parse(item)
				if !ok {
					return ConversionError{
						Key:   path,
						Type:  elemT,
						Index: key,
					}
				}
				out = append(out, ev)
			}
		default:
			ev, ok := parse(value)
			if !ok {
				return ConversionError{
					Key:   path,
					Type:  elemT,
					Index: key,
				}
			}
			out = append(out, ev)
		}
	}
	// v's type is exactly []T here (the dispatch switch guarantees it), and v
	// is settable (decode returns before reaching here for a field it cannot
	// set), so assign through a typed pointer to it: no reflect.ValueOf
	// escape and no Set assignability checks. The pointer comes from
	// UnsafeAddr rather than Addr().Interface(), which resolved the pointer
	// type on every call and measured a fifth of the whole assignment's cost.
	*(*[]T)(unsafe.Pointer(v.UnsafeAddr())) = out
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

// The error strings below are assembled by concatenation instead of
// fmt.Sprintf: %q is strconv.Quote and %d is utils.FormatInt, so the messages
// are byte-identical without the reflection-based formatter.
func (e ConversionError) Error() string {
	var output string

	if e.Index < 0 {
		output = "schema: error converting value for " + strconv.Quote(e.Key)
	} else {
		output = "schema: error converting value for index " +
			utils.FormatInt(int64(e.Index)) + " of " + strconv.Quote(e.Key)
	}

	if e.Err != nil {
		output += ". Details: " + e.Err.Error()
	}

	return output
}

// UnknownKeyError stores information about an unknown key in the source map.
type UnknownKeyError struct {
	Key string // key from the source map.
}

func (e UnknownKeyError) Error() string {
	return "schema: invalid path " + strconv.Quote(e.Key)
}

// EmptyFieldError stores information about an empty required field.
type EmptyFieldError struct {
	Key string // required key in the source map.
}

func (e EmptyFieldError) Error() string {
	return e.Key + " is empty"
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
	return s + " (and " + utils.FormatInt(int64(len(e)-1)) + " other errors)"
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

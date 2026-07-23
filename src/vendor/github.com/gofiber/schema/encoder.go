package schema

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"

	utils "github.com/gofiber/utils/v2"
)

type encoderFunc func(reflect.Value) string

// errNotStruct is returned by Encode for invalid sources; hoisted so the
// check does not allocate on every call.
var errNotStruct = errors.New("schema: interface must be a struct")

// errNilDst is returned by Encode when the destination map is nil, which
// would otherwise panic on the first map assignment.
var errNilDst = errors.New("schema: dst map must not be nil")

// Encoder encodes values from a struct into url.Values.
type Encoder struct {
	cache  *cache
	regenc map[reflect.Type]encoderFunc
	// encCache memoizes the per-struct-type encoding plan
	// (map[reflect.Type][]encField) so tags are parsed and encoder
	// functions resolved once per type instead of on every Encode call.
	encCache sync.Map
	// encGen is bumped before encCache is cleared on configuration changes;
	// structInfo snapshots it before building a plan and refuses to store
	// the plan if it changed, so a build racing a reconfiguration cannot
	// re-insert a stale plan after the clear.
	encGen atomic.Uint64
}

// encPlan tags a per-type encoding plan with the configuration generation it
// was built under: hit-validation ignores plans from older generations, so
// any Encode starting after a reconfiguration returns observes the new
// configuration even if a racing build stored a stale plan after the clear.
type encPlan struct {
	fields []encField
	gen    uint64
}

// encField is the precomputed encoding plan for one struct field.
type encField struct {
	name      string
	enc       encoderFunc // immediate encoder; nil for structs and slices
	elemEnc   encoderFunc // slice element encoder, when the field is a slice
	idx       int
	omitEmpty bool
	// recurseStructPtr marks pointer-to-struct fields without a custom
	// encoder: non-nil values are encoded by recursing into the element.
	recurseStructPtr bool
	isStruct         bool
	// nilAsNull marks pointer fields whose element has no immediate
	// encoder (structs recursed via recurseStructPtr, or unsupported
	// types): nil values encode as "null", matching the closure behavior
	// pointer fields with encodable elements get.
	nilAsNull bool
	// elemPtrNil marks slice fields whose element is a pointer type with no
	// encoder (e.g. []*Struct): nil elements encode as "null" (as they did
	// historically), while a non-nil such element is an error.
	elemPtrNil bool
}

// NewEncoder returns a new Encoder with defaults.
func NewEncoder() *Encoder {
	return &Encoder{cache: newCache(), regenc: make(map[reflect.Type]encoderFunc)}
}

// Encode encodes a struct into map[string][]string.
//
// Intended for use with url.Values.
func (e *Encoder) Encode(src interface{}, dst map[string][]string) (err error) {
	if dst == nil {
		return errNilDst
	}

	// Catch panics from reflection or user-registered encoders and return
	// them as an error instead of crashing the caller, mirroring Decode.
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("schema: panic while encoding: %v", r)
			}
		}
	}()

	v := reflect.ValueOf(src)

	return e.encode(v, dst)
}

// RegisterEncoder registers a converter for encoding a custom type.
func (e *Encoder) RegisterEncoder(value interface{}, encoder func(reflect.Value) string) {
	e.cache.l.Lock()
	e.regenc[reflect.TypeOf(value)] = encoder
	e.cache.l.Unlock()
	e.encGen.Add(1)
	e.encCache.Clear()
}

// SetAliasTag changes the tag used to locate custom field aliases.
// The default tag is "schema".
func (e *Encoder) SetAliasTag(tag string) {
	e.cache.l.Lock()
	e.cache.tag = tag
	e.cache.l.Unlock()
	e.encGen.Add(1)
	e.encCache.Clear()
}

// structInfo returns the cached encoding plan for struct type t, building it
// on first use. The build reads the tag and registered encoders under the
// configuration lock; the generation re-checks around the cache store keep a
// build racing a reconfiguration from inserting a stale plan.
func (e *Encoder) structInfo(t reflect.Type) []encField {
	gen := e.encGen.Load()
	if cached, ok := e.encCache.Load(t); ok {
		// Ignore plans built under an older configuration; fall through and
		// rebuild (the fresh plan overwrites the stale entry).
		if p := cached.(*encPlan); p.gen == gen {
			return p.fields
		}
	}
	e.cache.l.RLock()
	tag := e.cache.tag
	fields := make([]encField, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		name, opts := fieldAlias(sf, tag)
		if name == "-" {
			continue
		}
		ft := sf.Type
		f := encField{
			idx:       i,
			name:      name,
			omitEmpty: opts.Contains("omitempty"),
			recurseStructPtr: ft.Kind() == reflect.Ptr &&
				ft.Elem().Kind() == reflect.Struct &&
				!e.hasCustomEncoder(ft),
			enc: typeEncoder(ft, e.regenc),
		}
		if f.enc == nil {
			switch ft.Kind() {
			case reflect.Struct:
				f.isStruct = true
			case reflect.Slice:
				f.elemEnc = typeEncoder(ft.Elem(), e.regenc)
				if f.elemEnc == nil && ft.Elem().Kind() == reflect.Ptr {
					f.elemPtrNil = true
				}
			case reflect.Ptr:
				f.nilAsNull = true
			}
		}
		fields = append(fields, f)
	}
	e.cache.l.RUnlock()
	// Don't cache a plan whose inputs (tag, registered encoders) changed
	// while it was being built; the next call rebuilds it fresh. Even if a
	// stale plan slips in after the clear, its generation tag keeps it from
	// ever being served.
	if e.encGen.Load() == gen {
		e.encCache.Store(t, &encPlan{fields: fields, gen: gen})
	}
	return fields
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Func:
	case reflect.Map, reflect.Slice:
		return v.IsNil() || v.Len() == 0
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !isZero(v.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Struct:
		if v.CanInterface() {
			if iz, ok := v.Interface().(interface{ IsZero() bool }); ok {
				return iz.IsZero()
			}
		}
		for i := 0; i < v.NumField(); i++ {
			if !isZero(v.Field(i)) {
				return false
			}
		}
		return true
	}
	// Compare other types directly:
	return v.IsZero()
}

func (e *Encoder) encode(v reflect.Value, dst map[string][]string) error {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return errNotStruct
	}

	var errs MultiError

	fields := e.structInfo(v.Type())
	for i := range fields {
		f := &fields[i]
		fieldValue := v.Field(f.idx)

		// Encode struct pointer types if the field is a valid pointer and a struct.
		if f.recurseStructPtr && !fieldValue.IsNil() {
			if err := e.encode(fieldValue.Elem(), dst); err != nil {
				errs = setError(errs, fieldValue.Elem().Type().String(), err)
			}
			continue
		}

		// Encode non-slice types and custom implementations immediately.
		if f.enc != nil {
			if f.omitEmpty && isZero(fieldValue) {
				continue
			}
			dst[f.name] = append(dst[f.name], f.enc(fieldValue))
			continue
		}

		if f.nilAsNull && fieldValue.IsNil() {
			if f.omitEmpty {
				continue
			}
			dst[f.name] = append(dst[f.name], "null")
			continue
		}

		if f.isStruct {
			if err := e.encode(fieldValue, dst); err != nil {
				errs = setError(errs, fieldValue.Type().String(), err)
			}
			continue
		}

		// A non-slice field with no encoder (map, chan, array, or a non-nil
		// pointer to an unencodable type), or a slice whose element type is
		// itself unencodable and not a pointer (e.g. []Struct), cannot be
		// encoded — historically this errored unconditionally.
		if fieldValue.Kind() != reflect.Slice || (f.elemEnc == nil && !f.elemPtrNil) {
			errs = setError(errs, fieldValue.Type().String(), fmt.Errorf("schema: encoder not found for %v", fieldValue))
			continue
		}

		// Encode a slice. An empty slice has nothing to encode, so it is
		// skipped under omitempty (and otherwise emitted empty).
		n := fieldValue.Len()
		if n == 0 && f.omitEmpty {
			continue
		}

		values := make([]string, n)
		if f.elemEnc == nil {
			// Pointer elements with no encoder (elemPtrNil): nil encodes as
			// "null" (as historically), a non-nil such element is an error.
			bad := false
			for j := 0; j < n; j++ {
				if fieldValue.Index(j).IsNil() {
					values[j] = "null"
					continue
				}
				errs = setError(errs, fieldValue.Type().String(), fmt.Errorf("schema: encoder not found for %v", fieldValue))
				bad = true
				break
			}
			if bad {
				continue
			}
		} else {
			for j := 0; j < n; j++ {
				values[j] = f.elemEnc(fieldValue.Index(j))
			}
		}
		dst[f.name] = values
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// setError lazily allocates m and stores err under key, overwriting any
// previous entry (matching the historical encoder error semantics).
func setError(m MultiError, key string, err error) MultiError {
	if m == nil {
		m = make(MultiError)
	}
	m[key] = err
	return m
}

func (e *Encoder) hasCustomEncoder(t reflect.Type) bool {
	_, exists := e.regenc[t]
	return exists
}

func typeEncoder(t reflect.Type, reg map[reflect.Type]encoderFunc) encoderFunc {
	if f, ok := reg[t]; ok {
		return f
	}

	switch t.Kind() {
	case reflect.Bool:
		return encodeBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return encodeInt
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return encodeUint
	case reflect.Float32:
		return encodeFloat32
	case reflect.Float64:
		return encodeFloat64
	case reflect.Ptr:
		f := typeEncoder(t.Elem(), reg)
		if f == nil {
			// No encoder for the element: report unsupported instead of
			// returning a closure that would panic on non-nil values.
			// Nil handling for such fields is done by encField.nilAsNull.
			return nil
		}
		return func(v reflect.Value) string {
			if v.IsNil() {
				return "null"
			}
			return f(v.Elem())
		}
	case reflect.String:
		return encodeString
	default:
		return nil
	}
}

func encodeBool(v reflect.Value) string {
	return strconv.FormatBool(v.Bool())
}

func encodeInt(v reflect.Value) string {
	return utils.FormatInt(v.Int())
}

func encodeUint(v reflect.Value) string {
	return utils.FormatUint(v.Uint())
}

func encodeFloat(v reflect.Value, bits int) string {
	return strconv.FormatFloat(v.Float(), 'f', 6, bits)
}

func encodeFloat32(v reflect.Value) string {
	return encodeFloat(v, 32)
}

func encodeFloat64(v reflect.Value) string {
	return encodeFloat(v, 64)
}

func encodeString(v reflect.Value) string {
	return v.String()
}

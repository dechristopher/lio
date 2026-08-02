// Copyright 2012 The Gorilla Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package schema

import (
	"errors"
	"maps"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	utils "github.com/gofiber/utils/v2"
	utilstrings "github.com/gofiber/utils/v2/strings"
	"github.com/gofiber/utils/v2/swar"
)

const maxParserIndex = 1000

var (
	errInvalidPath   = errors.New("schema: invalid path")
	errIndexTooLarge = errors.New("schema: index exceeds parser limit")
)

// newCache returns a new cache.
func newCache() *cache {
	c := cache{
		tag: "schema",
	}
	return &c
}

// cache caches meta-data about a struct.
type cache struct {
	l sync.RWMutex // serializes configuration writes (tag, regconv)
	m sync.Map     // map[reflect.Type]cacheEntry
	// regconv holds the registered converters as an immutable map published
	// atomically: registerConverter replaces the whole map (copy-on-write
	// under l), so readers never touch a map that is being written.
	regconv atomic.Pointer[map[reflect.Type]Converter]
	tag     string
	// gen is bumped (under l) before m is cleared on configuration changes;
	// cached entries are tagged with the generation they were built under
	// and ignored on mismatch, so any call starting after a reconfiguration
	// returns observes the new configuration even if a racing build stored
	// a stale entry after the clear.
	gen atomic.Uint64
}

// cacheEntry tags a structInfo with the configuration generation it was
// built under; entries are stored by pointer in c.m (matching the encoder's
// encPlan pattern).
type cacheEntry struct {
	info *structInfo
	gen  uint64
}

// registerConverter registers a converter function for a custom type.
func (c *cache) registerConverter(value interface{}, converterFunc Converter) {
	c.l.Lock()
	next := make(map[reflect.Type]Converter)
	if prev := c.regconv.Load(); prev != nil {
		maps.Copy(next, *prev)
	}
	next[reflect.TypeOf(value)] = converterFunc
	c.regconv.Store(&next)
	c.reset()
	c.l.Unlock()
}

// parsePath parses a path in dotted notation verifying that it is a valid
// path to a struct field.
//
// It returns "path parts" which contain indices to fields to be used by
// reflect.Value.FieldByString(). Multiple parts are required for slices of
// structs.
func (c *cache) parsePath(p string, t reflect.Type) ([]pathPart, error) {
	if t.Kind() != reflect.Struct {
		return nil, errInvalidPath
	}
	return c.parsePathInfo(p, c.get(t))
}

// parsePathInfo is parsePath with the root struct's info already resolved,
// letting Decode look it up once per call instead of once per key. The
// parsed-path cache lives on that structInfo, keyed by the plain path
// string, which hashes much cheaper than a composite key.
func (c *cache) parsePathInfo(p string, rootInfo *structInfo) ([]pathPart, error) {
	if cached, ok := rootInfo.paths.Load(p); ok {
		return cached.([]pathPart), nil
	}

	struc := rootInfo
	var t reflect.Type
	var field *fieldInfo
	var index64 int64
	var parts []pathPart
	var hops []pathHop
	for keyStart := 0; ; {
		keyEnd, segment, err := nextPathSegment(p, keyStart)
		if err != nil {
			return nil, errInvalidPath
		}
		if field = struc.get(segment); field == nil {
			return nil, errInvalidPath
		}
		// Valid field. Append the hop; the field's index chain was resolved
		// when the structInfo was built, so the decoder walks plain indices
		// instead of repeating FieldByName lookups on every Decode call.
		hops = append(hops, pathHop{index: field.index, ensure: struc.anonymousPtrFields})
		if field.isSliceOfStructs && !field.isMultipart && (!field.unmarshalerInfo.IsValid || (field.unmarshalerInfo.IsValid && field.unmarshalerInfo.IsSliceElement)) {
			// Parse a special case: slices of structs.
			// i+1 must be the slice index.
			//
			// Now that struct can implements TextUnmarshaler interface,
			// we don't need to force the struct's fields to appear in the path.
			// So checking i+2 is not necessary anymore.
			// We can skip this part if the type is multipart.FileHeader. It is another special case too.
			keyStart = keyEnd + 1
			if keyStart >= len(p) {
				return nil, errInvalidPath
			}
			keyEnd, segment, err = nextPathSegment(p, keyStart)
			if err != nil {
				return nil, errInvalidPath
			}
			if index64, err = utils.ParseInt(segment); err != nil {
				return nil, errInvalidPath
			}
			if index64 > maxParserIndex {
				return nil, errIndexTooLarge
			}
			parts = append(parts, pathPart{
				hops:  hops,
				field: field,
				index: int(index64),
			})
			hops = nil

			// Get the next struct type, dropping ptrs.
			if field.typ.Kind() == reflect.Ptr {
				t = field.typ.Elem()
			} else {
				t = field.typ
			}
			if t.Kind() == reflect.Slice {
				t = t.Elem()
				if t.Kind() == reflect.Ptr {
					t = t.Elem()
				}
			}
		} else if field.typ.Kind() == reflect.Ptr {
			t = field.typ.Elem()
		} else {
			t = field.typ
		}

		if keyEnd == len(p) {
			break
		}
		keyStart = keyEnd + 1
		if keyStart >= len(p) {
			return nil, errInvalidPath
		}
		if t.Kind() != reflect.Struct {
			return nil, errInvalidPath
		}
		struc = c.get(t)
	}
	// Add the remaining. A part without hops means the path terminated at a
	// slice index ("a.0"), so the decoder receives a slice element there.
	parts = append(parts, pathPart{
		hops:  hops,
		field: field,
		index: -1,
		elem:  len(hops) == 0,
	})

	// Detach the key: callers may pass strings aliasing reused request buffers.
	if cached, loaded := rootInfo.paths.LoadOrStore(strings.Clone(p), parts); loaded {
		return cached.([]pathPart), nil
	}

	return parts, nil
}

// dotBroadcast is the SWAR needle for '.'; hoisted so the word loop in
// nextPathSegment pays no per-call broadcast cost.
var dotBroadcast = swar.Broadcast('.')

func nextPathSegment(path string, start int) (int, string, error) {
	end := start
	for end+swar.WordLen <= len(path) {
		if m := swar.ZeroLanes(swar.Load8(path, end) ^ dotBroadcast); m != 0 {
			end += swar.FirstLane(m)
			if start == end {
				return 0, "", errInvalidPath
			}
			return end, path[start:end], nil
		}
		end += swar.WordLen
	}
	for end < len(path) && path[end] != '.' {
		end++
	}
	if start == end {
		return 0, "", errInvalidPath
	}
	return end, path[start:end], nil
}

// get returns a cached structInfo, creating it if necessary.
func (c *cache) get(t reflect.Type) *structInfo {
	gen := c.gen.Load()
	if v, ok := c.m.Load(t); ok {
		// Ignore entries built under an older configuration: a build racing
		// a reconfiguration may store one after the clear. Hit-validation
		// guarantees that any call starting after the reconfiguration
		// returned observes the new configuration (a call already in flight
		// during the reconfiguration may still briefly use old metadata,
		// which is inherent to concurrent reconfiguration).
		if e := v.(*cacheEntry); e.gen == gen {
			return e.info
		}
	}
	info := c.create(t, "")
	if c.gen.Load() == gen {
		c.m.Store(t, &cacheEntry{info: info, gen: gen})
	}
	// If the configuration changed while building, serve the result once
	// without caching it (or with a stale tag that hit-validation ignores);
	// the next call rebuilds fresh.
	return info
}

// reset clears cached metadata and must be called with c.l held. Parsed
// path caches live on the structInfos, so dropping them drops those too.
func (c *cache) reset() {
	c.gen.Add(1)
	c.m.Clear()
}

// aliasTag returns the configured tag name under the configuration lock, so
// metadata builds racing SetAliasTag read a consistent value.
func (c *cache) aliasTag() string {
	c.l.RLock()
	tag := c.tag
	c.l.RUnlock()
	return tag
}

// create creates a structInfo with meta-data about a struct.
func (c *cache) create(t reflect.Type, parentAlias string) *structInfo {
	// Snapshot the alias tag once so every field of this type is analyzed
	// under a consistent configuration and we don't churn the config lock
	// per field.
	tag := c.aliasTag()
	info := &structInfo{}
	var anonymousInfos []*structInfo
	var anonymousIdx [][]int
	for i := 0; i < t.NumField(); i++ {
		structField := t.Field(i)
		// Only exported anonymous pointers can be allocated; unexported ones
		// are not settable and Set would panic.
		if structField.Anonymous && structField.Type.Kind() == reflect.Ptr && structField.IsExported() {
			info.anonymousPtrFields = append(info.anonymousPtrFields, i)
		}
		if f := c.createField(structField, parentAlias, tag); f != nil {
			f.index = structField.Index
			info.fields = append(info.fields, f)
			if ft := indirectType(f.typ); ft.Kind() == reflect.Struct && f.isAnonymous {
				anonymousInfos = append(anonymousInfos, c.create(ft, f.canonicalAlias))
				anonymousIdx = append(anonymousIdx, structField.Index)
			}
		}
	}
	for i, a := range anonymousInfos {
		others := []*structInfo{info}
		others = append(others, anonymousInfos[:i]...)
		others = append(others, anonymousInfos[i+1:]...)
		for _, f := range a.fields {
			if !containsAlias(others, f.alias) {
				// Copy the promoted field so its index chain can be prefixed
				// with the embedded field's index; the original stays valid
				// for the embedded type's own structInfo.
				pf := *f
				pf.index = append(append(make([]int, 0, len(anonymousIdx[i])+len(f.index)), anonymousIdx[i]...), f.index...)
				info.fields = append(info.fields, &pf)
			}
		}
	}
	info.fieldsByName = make(map[string]*fieldInfo, len(info.fields))
	for _, field := range info.fields {
		if _, exists := info.fieldsByName[field.aliasLower]; !exists {
			info.fieldsByName[field.aliasLower] = field
		}
	}
	info.requiredFields = c.buildRequiredFields(info)
	// The setDefaults walk also allocates nil anonymous embedded pointers,
	// so it can only be skipped when neither defaults nor such pointers
	// exist anywhere in the tree.
	info.needsDefaultsWalk = c.needsDefaultsWalk(t, tag, map[reflect.Type]bool{})
	return info
}

// needsDefaultsWalk reports whether the setDefaults walk can have any effect
// on the struct tree rooted at t: it declares a default tag option, or has an
// (exported) anonymous pointer field the walk allocates, anywhere in the
// tree. visited guards against recursive types. tag is the alias tag
// snapshot for this build, so no per-field config lock is taken.
func (c *cache) needsDefaultsWalk(t reflect.Type, tag string, visited map[reflect.Type]bool) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || visited[t] {
		return false
	}
	visited[t] = true
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// Mirror the anonymousPtrFields filter: only exported anonymous
		// pointers are ever allocated, so only they justify the walk.
		if field.Anonymous && field.Type.Kind() == reflect.Ptr && field.IsExported() {
			return true
		}
		alias, options := fieldAlias(field, tag)
		if alias == "-" {
			continue
		}
		if options.getDefaultOptionValue() != "" {
			return true
		}
		if c.needsDefaultsWalk(field.Type, tag, visited) {
			return true
		}
	}
	return false
}

// createField creates a fieldInfo for the given field. tag is the alias tag
// snapshot for the enclosing type's build.
func (c *cache) createField(field reflect.StructField, parentAlias, tag string) *fieldInfo {
	alias, options := fieldAlias(field, tag)
	if alias == "-" {
		// Ignore this field.
		return nil
	}
	canonicalAlias := alias
	if parentAlias != "" {
		canonicalAlias = parentAlias + "." + alias
	}
	// Check if the type is supported and don't cache it if not.
	// First let's get the basic type.
	isSlice, isStruct := false, false
	ft := field.Type
	m := isTextUnmarshaler(reflect.Zero(ft))
	if ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}
	if isSlice = ft.Kind() == reflect.Slice; isSlice {
		ft = ft.Elem()
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
	}
	if ft.Kind() == reflect.Array {
		ft = ft.Elem()
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
	}
	if isStruct = ft.Kind() == reflect.Struct; !isStruct {
		if c.converter(ft) == nil && getBuiltinConverter(ft.Kind()) == nil {
			// Type is not supported.
			return nil
		}
	}

	// Reuse the unmarshaler facts when the successive type unwrappings land
	// on the same type (the common non-pointer, non-slice case).
	derefT := indirectType(field.Type)
	derefU := m
	if derefT != field.Type {
		derefU = isTextUnmarshaler(reflect.Zero(derefT))
	}
	elemU := derefU
	if ft != derefT {
		elemU = isTextUnmarshaler(reflect.Zero(ft))
	}

	return &fieldInfo{
		typ:              field.Type,
		name:             field.Name,
		alias:            alias,
		aliasLower:       utilstrings.ToLower(alias),
		canonicalAlias:   canonicalAlias,
		unmarshalerInfo:  m,
		derefUnmarshaler: derefU,
		elemUnmarshaler:  elemU,
		isMultipart:      isMultipartField(field.Type),
		isSliceOfStructs: isSlice && isStruct,
		isAnonymous:      field.Anonymous,
		isRequired:       options.Contains("required"),
		defaultValue:     options.getDefaultOptionValue(),
	}
}

// converter returns the converter for a type.
func (c *cache) converter(t reflect.Type) Converter {
	reg := c.regconv.Load()
	if reg == nil {
		return nil
	}
	return (*reg)[t]
}

// ----------------------------------------------------------------------------

type structInfo struct {
	fields             []*fieldInfo
	fieldsByName       map[string]*fieldInfo
	anonymousPtrFields []int
	requiredFields     map[string][]fieldWithPrefix
	// paths caches parsed paths rooted at this struct type
	// (map[string][]pathPart); keys are cloned so they never alias reused
	// request buffers.
	paths sync.Map
	// needsDefaultsWalk reports whether the setDefaults walk can have any
	// effect on this struct tree: it is set when a default tag option or an
	// anonymous embedded pointer field (which the walk allocates) exists
	// anywhere in the tree, letting the decoder skip the walk otherwise.
	needsDefaultsWalk bool
}

func (i *structInfo) get(alias string) *fieldInfo {
	aliasKey := utilstrings.ToLower(alias)
	if field, ok := i.fieldsByName[aliasKey]; ok {
		return field
	}
	return nil
}

func (c *cache) buildRequiredFields(info *structInfo) map[string][]fieldWithPrefix {
	var requiredFields map[string][]fieldWithPrefix
	for _, field := range info.fields {
		if field.typ.Kind() == reflect.Struct {
			nested := c.get(field.typ)
			for _, prefix := range field.paths("") {
				nestedPrefix := prefix + "."
				for key, fields := range nested.requiredFields {
					requiredKey := field.canonicalAlias + "." + key
					for _, nestedField := range fields {
						requiredFields = appendRequiredField(requiredFields, requiredKey,
							newFieldWithPrefix(nestedField.fieldInfo, nestedPrefix+nestedField.prefix))
					}
				}
			}
		}
		if field.isRequired {
			requiredFields = appendRequiredField(requiredFields, field.canonicalAlias,
				newFieldWithPrefix(field, ""))
		}
	}
	return requiredFields
}

func containsAlias(infos []*structInfo, alias string) bool {
	aliasKey := utilstrings.ToLower(alias)
	for _, info := range infos {
		if _, ok := info.fieldsByName[aliasKey]; ok {
			return true
		}
	}
	return false
}

type fieldInfo struct {
	typ reflect.Type
	// index is the field index chain relative to the struct type whose
	// structInfo holds this fieldInfo; promoted fields carry the full chain
	// through the embedded structs (a copy is made per promotion level).
	index []int
	// name is the field name in the struct.
	name  string
	alias string
	// aliasLower is the pre-computed lowercase alias for fast lookups.
	aliasLower string
	// canonicalAlias is almost the same as the alias, but is prefixed with
	// an embedded struct field alias in dotted notation if this field is
	// promoted from the struct.
	// For instance, if the alias is "N" and this field is an embedded field
	// in a struct "X", canonicalAlias will be "X.N".
	canonicalAlias string
	// unmarshalerInfo contains information regarding the
	// encoding.TextUnmarshaler implementation of the field type.
	unmarshalerInfo unmarshaler
	// derefUnmarshaler caches the encoding.TextUnmarshaler information for
	// the field type after one pointer dereference, which is what the
	// decoder sees after walking to the field. Only the type-level flags are
	// meaningful; the decoder binds instances to live values itself.
	derefUnmarshaler unmarshaler
	// elemUnmarshaler is like derefUnmarshaler but for the fully unwrapped
	// slice element type; the decoder uses it when a path terminates at a
	// slice index (e.g. "a.0") and the value at hand is an element rather
	// than the slice field itself.
	elemUnmarshaler unmarshaler
	// isMultipart indicates whether the field type is one of the supported
	// multipart file header shapes, precomputed so the decoder can skip the
	// type comparisons on every other field.
	isMultipart bool
	// isSliceOfStructs indicates if the field type is a slice of structs.
	isSliceOfStructs bool
	// isAnonymous indicates whether the field is embedded in the struct.
	isAnonymous  bool
	isRequired   bool
	defaultValue string
}

func (f *fieldInfo) paths(prefix string) []string {
	if f.alias == f.canonicalAlias {
		return []string{prefix + f.alias}
	}
	return []string{prefix + f.alias, prefix + f.canonicalAlias}
}

type pathPart struct {
	field *fieldInfo
	hops  []pathHop // path to the field: walks structs using field indices.
	index int       // struct index in slices of structs.
	// elem marks a terminal part whose path ended at a slice index ("a.0"):
	// the decoder's value is then an element of the slice field rather than
	// the field itself.
	elem bool
}

// pathHop describes one named-field lookup along a path. index is the field
// index chain relative to the struct at this level (more than one element
// when the field is promoted from embedded structs), and ensure lists the
// anonymous pointer fields of that struct which must be allocated before the
// walk so promoted fields stay reachable.
type pathHop struct {
	index  []int
	ensure []int
}

// ----------------------------------------------------------------------------

func indirectType(typ reflect.Type) reflect.Type {
	if typ.Kind() == reflect.Ptr {
		return typ.Elem()
	}
	return typ
}

// fieldAlias parses a field tag to get a field alias.
func fieldAlias(field reflect.StructField, tagName string) (alias string, options tagOptions) {
	if tag := field.Tag.Get(tagName); tag != "" {
		alias, options = parseTag(tag)
	}
	if alias == "" {
		alias = field.Name
	}
	return alias, options
}

// tagOptions is the string following a comma in a struct field's tag, or
// the empty string. It does not include the leading comma. Keeping the raw
// comma-separated string avoids the []string allocation of strings.Split on
// hot paths (the encoder parses tags on every Encode call).
type tagOptions string

// parseTag splits a struct field's url tag into its name and comma-separated
// options.
func parseTag(tag string) (string, tagOptions) {
	if idx := strings.IndexByte(tag, ','); idx != -1 {
		return tag[:idx], tagOptions(tag[idx+1:])
	}
	return tag, ""
}

// Contains checks whether the tagOptions contains the specified option.
func (o tagOptions) Contains(option string) bool {
	if o == "" {
		return false
	}
	for s := range strings.SplitSeq(string(o), ",") {
		if s == option {
			return true
		}
	}
	return false
}

func (o tagOptions) getDefaultOptionValue() string {
	if o == "" {
		return ""
	}
	for s := range strings.SplitSeq(string(o), ",") {
		if value, ok := strings.CutPrefix(s, "default:"); ok {
			return value
		}
	}
	return ""
}

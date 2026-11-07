// Package schema is a minimal JSON Schema (draft 2020-12) validator for the
// keyword subset used by contract/events.schema.json. Keywords outside
// SupportedKeywords are rejected at compile time, so the schema cannot silently
// rely on something this validator ignores.
package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SupportedKeywords lists every keyword the validator understands.
var SupportedKeywords = map[string]bool{
	"$schema": true, "$id": true, "$ref": true, "$defs": true,
	"title": true, "description": true,
	"type": true, "enum": true, "properties": true, "required": true,
	"additionalProperties": true, "items": true, "minItems": true,
	"minLength": true, "maxLength": true, "pattern": true, "format": true,
	"minimum": true, "maximum": true,
}

// Schema is a compiled schema.
type Schema struct {
	root *node
	defs map[string]*node
}

type node struct {
	ref        string
	types      []string
	enum       []any
	properties map[string]*node
	required   []string
	addlBool   *bool
	addlSchema *node
	items      *node
	minItems   *int
	minLength  *int
	maxLength  *int
	pattern    *regexp.Regexp
	format     string
	minimum    *float64
	maximum    *float64
}

// Compile parses a schema document.
func Compile(data []byte) (*Schema, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	s := &Schema{defs: map[string]*node{}}
	if defs, ok := raw["$defs"].(map[string]any); ok {
		for name, d := range defs {
			m, ok := d.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("schema: $defs/%s is not an object", name)
			}
			n, err := compileNode(m, "#/$defs/"+name)
			if err != nil {
				return nil, err
			}
			s.defs["#/$defs/"+name] = n
		}
	}
	root, err := compileNode(raw, "#")
	if err != nil {
		return nil, err
	}
	s.root = root
	// Check every $ref resolves.
	var check func(n *node) error
	seen := map[*node]bool{}
	check = func(n *node) error {
		if n == nil || seen[n] {
			return nil
		}
		seen[n] = true
		if n.ref != "" {
			if _, ok := s.defs[n.ref]; !ok {
				return fmt.Errorf("schema: unresolved $ref %q", n.ref)
			}
		}
		for _, p := range n.properties {
			if err := check(p); err != nil {
				return err
			}
		}
		if err := check(n.addlSchema); err != nil {
			return err
		}
		return check(n.items)
	}
	if err := check(root); err != nil {
		return nil, err
	}
	for _, d := range s.defs {
		if err := check(d); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func compileNode(m map[string]any, path string) (*node, error) {
	n := &node{}
	for k := range m {
		if !SupportedKeywords[k] {
			return nil, fmt.Errorf("schema: unsupported keyword %q at %s", k, path)
		}
	}
	if r, ok := m["$ref"].(string); ok {
		if !strings.HasPrefix(r, "#/$defs/") {
			return nil, fmt.Errorf("schema: only local #/$defs refs are supported, got %q at %s", r, path)
		}
		n.ref = r
	}
	switch t := m["type"].(type) {
	case string:
		n.types = []string{t}
	case []any:
		for _, v := range t {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("schema: bad type at %s", path)
			}
			n.types = append(n.types, s)
		}
	case nil:
	default:
		return nil, fmt.Errorf("schema: bad type at %s", path)
	}
	if e, ok := m["enum"].([]any); ok {
		n.enum = e
	}
	if p, ok := m["properties"].(map[string]any); ok {
		n.properties = map[string]*node{}
		for name, v := range p {
			pm, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("schema: property %s at %s is not an object", name, path)
			}
			c, err := compileNode(pm, path+"/properties/"+name)
			if err != nil {
				return nil, err
			}
			n.properties[name] = c
		}
	}
	if r, ok := m["required"].([]any); ok {
		for _, v := range r {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("schema: bad required at %s", path)
			}
			n.required = append(n.required, s)
		}
	}
	switch a := m["additionalProperties"].(type) {
	case bool:
		n.addlBool = &a
	case map[string]any:
		c, err := compileNode(a, path+"/additionalProperties")
		if err != nil {
			return nil, err
		}
		n.addlSchema = c
	}
	if it, ok := m["items"].(map[string]any); ok {
		c, err := compileNode(it, path+"/items")
		if err != nil {
			return nil, err
		}
		n.items = c
	}
	intKw := func(k string) (*int, error) {
		v, ok := m[k]
		if !ok {
			return nil, nil
		}
		f, ok := v.(float64)
		if !ok || f != math.Trunc(f) || f < 0 {
			return nil, fmt.Errorf("schema: %s must be a non-negative integer at %s", k, path)
		}
		i := int(f)
		return &i, nil
	}
	var err error
	if n.minItems, err = intKw("minItems"); err != nil {
		return nil, err
	}
	if n.minLength, err = intKw("minLength"); err != nil {
		return nil, err
	}
	if n.maxLength, err = intKw("maxLength"); err != nil {
		return nil, err
	}
	if p, ok := m["pattern"].(string); ok {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("schema: pattern at %s: %w", path, err)
		}
		n.pattern = re
	}
	if f, ok := m["format"].(string); ok {
		switch f {
		case "date-time", "date":
		default:
			return nil, fmt.Errorf("schema: unsupported format %q at %s", f, path)
		}
		n.format = f
	}
	if v, ok := m["minimum"].(float64); ok {
		n.minimum = &v
	}
	if v, ok := m["maximum"].(float64); ok {
		n.maximum = &v
	}
	return n, nil
}

// ValidationError lists every violation found.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return "schema validation failed: " + strings.Join(e.Problems, "; ")
}

// Validate checks a JSON document against the schema.
func (s *Schema) Validate(doc []byte) error {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return &ValidationError{Problems: []string{"invalid JSON: " + err.Error()}}
	}
	var probs []string
	s.validate(s.root, v, "", &probs, 0)
	if len(probs) > 0 {
		sort.Strings(probs)
		return &ValidationError{Problems: probs}
	}
	return nil
}

func (s *Schema) validate(n *node, v any, path string, probs *[]string, depth int) {
	if depth > 64 {
		*probs = append(*probs, path+": too deep")
		return
	}
	at := path
	if at == "" {
		at = "/"
	}
	fail := func(format string, args ...any) {
		*probs = append(*probs, at+": "+fmt.Sprintf(format, args...))
	}
	if n.ref != "" {
		s.validate(s.defs[n.ref], v, path, probs, depth+1)
	}
	if len(n.types) > 0 {
		ok := false
		for _, t := range n.types {
			if typeMatches(t, v) {
				ok = true
				break
			}
		}
		if !ok {
			fail("expected type %s", strings.Join(n.types, "|"))
			return
		}
	}
	if n.enum != nil {
		ok := false
		for _, e := range n.enum {
			if jsonEqual(e, v) {
				ok = true
				break
			}
		}
		if !ok {
			fail("value not in enum")
		}
	}
	switch x := v.(type) {
	case map[string]any:
		for _, r := range n.required {
			if _, ok := x[r]; !ok {
				fail("missing required property %q", r)
			}
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := path + "/" + k
			if p, ok := n.properties[k]; ok {
				s.validate(p, x[k], child, probs, depth+1)
				continue
			}
			if n.addlSchema != nil {
				s.validate(n.addlSchema, x[k], child, probs, depth+1)
			} else if n.addlBool != nil && !*n.addlBool {
				fail("unknown property %q", k)
			}
		}
	case []any:
		if n.minItems != nil && len(x) < *n.minItems {
			fail("expected at least %d items", *n.minItems)
		}
		if n.items != nil {
			for i, it := range x {
				s.validate(n.items, it, fmt.Sprintf("%s/%d", path, i), probs, depth+1)
			}
		}
	case string:
		l := len([]rune(x))
		if n.minLength != nil && l < *n.minLength {
			fail("shorter than %d", *n.minLength)
		}
		if n.maxLength != nil && l > *n.maxLength {
			fail("longer than %d", *n.maxLength)
		}
		if n.pattern != nil && !n.pattern.MatchString(x) {
			fail("does not match pattern %s", n.pattern)
		}
		switch n.format {
		case "date-time":
			if _, err := time.Parse(time.RFC3339, x); err != nil {
				fail("not an RFC 3339 date-time")
			}
		case "date":
			if _, err := time.Parse(time.DateOnly, x); err != nil {
				fail("not a date")
			}
		}
	case json.Number:
		f, _ := x.Float64()
		if n.minimum != nil && f < *n.minimum {
			fail("less than %v", *n.minimum)
		}
		if n.maximum != nil && f > *n.maximum {
			fail("greater than %v", *n.maximum)
		}
	}
}

func typeMatches(t string, v any) bool {
	switch t {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "null":
		return v == nil
	case "number":
		_, ok := v.(json.Number)
		return ok
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		if _, err := n.Int64(); err == nil {
			return true
		}
		f, err := n.Float64()
		return err == nil && f == math.Trunc(f)
	}
	return false
}

func jsonEqual(schemaVal, docVal any) bool {
	switch a := schemaVal.(type) {
	case float64:
		n, ok := docVal.(json.Number)
		if !ok {
			return false
		}
		f, err := n.Float64()
		return err == nil && f == a
	case string, bool, nil:
		return schemaVal == docVal
	}
	return false
}

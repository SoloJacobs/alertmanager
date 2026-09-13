// config_library.go is pure: it holds no integration information. It must not
// depend on any integration specific data either.
package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type Secret string

// HTTPConfig is the one global shared by every integration, so its global key
// carries no prefix.
type HTTPConfig struct {
	ProxyURL        string `yaml:"proxy_url,omitempty"`
	BearerToken     Secret `yaml:"bearer_token,omitempty"`
	FollowRedirects *bool  `yaml:"follow_redirects,omitempty"`
}

type Notifier interface {
	Notify(context.Context) error
}

// Parser turns the merged config into effective values, reporting as it goes
// rather than stopping at the first problem. It holds the config, so a step is
// given the key it reads and finds the value itself.
type Parser struct {
	errs   Errors
	fields map[string]reflect.Value
}

func newParser(errs Errors, conf any) *Parser {
	p := &Parser{errs: errs, fields: map[string]reflect.Value{}}
	v := reflect.ValueOf(conf).Elem()
	for i := 0; i < v.NumField(); i++ {
		key, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("yaml"), ",")
		p.fields[key] = v.Field(i)
	}
	return p
}

// field resolves a key to the config field it was decoded into. A missing key
// or a mismatched type is a bug in the integration, not in the config file.
func field[T any](p *Parser, key string) *T {
	v, ok := p.fields[key]
	if !ok {
		panic(fmt.Sprintf("library: no config field for key %q", key))
	}
	value, ok := v.Interface().(*T)
	if !ok {
		panic(fmt.Sprintf("library: key %q is %s, not %T", key, v.Type(), (*T)(nil)))
	}
	return value
}

// Source is the effective form of a key and the file it can be read from
// instead. It is opaque, so the notifier cannot tell which of the two it came
// from and cannot forget to handle one of them.
type Source[T ~string] struct {
	value T
	path  string
}

func (s Source[T]) Get() (T, error) {
	if s.path == "" {
		return s.value, nil
	}

	b, err := os.ReadFile(s.path)
	if err != nil {
		var zero T
		return zero, err
	}
	return T(strings.TrimSpace(string(b))), nil
}

// pairSource reads a key and the file it can come from instead, reporting when
// both are set and saying whether either was.
func pairSource[T ~string](p *Parser, key, fileKey string) (Source[T], bool) {
	value := field[T](p, key)

	var file *string
	if fileKey != "" {
		file = field[string](p, fileKey)
	}

	switch {
	case value != nil && file != nil:
		p.errs.Addf("at most one of %s & %s must be configured", key, fileKey)
		return Source[T]{}, true
	case value != nil:
		return Source[T]{value: *value}, true
	case file != nil:
		return Source[T]{path: *file}, true
	}
	return Source[T]{}, false
}

// ValueOrFile combines a key with its file counterpart. Both being set is an
// error, and so is neither, so the Source it writes always has one of them.
func ValueOrFile[T ~string](p *Parser, key, fileKey string, into *Source[T]) {
	s, ok := pairSource[T](p, key, fileKey)
	if !ok {
		p.errs.Addf("one of %s & %s must be configured", key, fileKey)
		return
	}
	*into = s
}

// alt is one alternative, already read, built by Alt.
type alt[A any] struct {
	keys  string
	value A
	set   bool
}

// Alt reads one alternative of a OneOf. It takes the integration's own
// constructor, so alternatives that are not interchangeable can stay distinct
// types instead of collapsing into one here.
func Alt[T ~string, A any](p *Parser, key, fileKey string, make func(Source[T]) A) alt[A] {
	keys := key
	if fileKey != "" {
		keys += "/" + fileKey
	}

	s, ok := pairSource[T](p, key, fileKey)
	if !ok {
		return alt[A]{keys: keys}
	}
	return alt[A]{keys: keys, value: make(s), set: true}
}

// OneOf picks the single alternative that was configured.
func OneOf[A any](p *Parser, into *A, alts ...alt[A]) {
	var (
		chosen []string
		value  A
		all    []string
	)
	for _, a := range alts {
		all = append(all, a.keys)
		if a.set {
			chosen = append(chosen, a.keys)
			value = a.value
		}
	}

	switch len(chosen) {
	case 1:
		*into = value
	case 0:
		p.errs.Addf("one of %s must be configured", strings.Join(all, ", "))
	default:
		p.errs.Addf("at most one of %s must be configured", strings.Join(chosen, ", "))
	}
}

// Enum takes a key that has to be one of a fixed set.
func Enum[T ~string](p *Parser, key string, fallback T, allowed []T, into *T) {
	Or(p, key, fallback, into)
	if !slices.Contains(allowed, *into) {
		p.errs.Addf("%s must be one of %v", key, allowed)
	}
}

// Or takes the configured value, or the default when the key was left out.
func Or[T any](p *Parser, key string, fallback T, into *T) {
	*into = fallback
	if value := field[T](p, key); value != nil {
		*into = *value
	}
}

// Errorf reports something no single step can decide on its own.
func (p *Parser) Errorf(format string, args ...any) {
	p.errs.Addf(format, args...)
}

// Required takes a key that has to be set to something.
func Required[T ~string](p *Parser, key string, into *T) {
	value := field[T](p, key)
	if value == nil || *value == "" {
		p.errs.Addf("%s is mandatory", key)
		return
	}
	*into = *value
}

// Build is a registered integration with its config type erased. Register is
// the only way to make one.
type Build func(global, receiver *yaml.Node) (Notifier, error)

// Register closes over the config type of one integration, so Main can drive
// every integration through the same steps without naming any of them.
// Register takes the pipeline one integration goes through, in the order the
// library runs it.
func Register[C any](
	extractGlobals func(*Globals) C,
	groups [][]string,
	parse func(*Parser) Notifier,
) Build {
	var zero C
	checkOptional(zero)

	merger := NewMerger[C](groups)

	return func(global, receiver *yaml.Node) (Notifier, error) {
		var errs Errors

		g := extractGlobals(&Globals{node: global, errs: &errs})

		var c C
		if receiver != nil {
			errs.Add(receiver.Decode(&c))
		}

		// Both halves are read before this check, so both failures are
		// reported, but a partial config is not worth merging or parsing.
		if err := errs.Err(); err != nil {
			return nil, err
		}

		c = merger.Merge(g, c)

		p := newParser(errs, &c)
		n := parse(p)

		if err := p.errs.Err(); err != nil {
			return nil, err
		}
		return n, nil
	}
}

func Main(s string, integrations map[string]Build) []Result {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(s), &node); err != nil {
		fmt.Println(err)
		return nil
	}

	results := load(&node, integrations)
	for _, r := range results {
		i := r.Integration
		if r.Err != nil {
			fmt.Printf("%s %s[%d]: %v\n", i.Receiver, i.Name, i.Index, r.Err)
			continue
		}
		fmt.Printf("%s %s[%d]: ok\n", i.Receiver, i.Name, i.Index)
	}
	return results
}

type Integration struct {
	Name     string
	Receiver string
	Index    int
	Notifier Notifier
}

type Result struct {
	Integration Integration
	Err         error
}

func load(node *yaml.Node, integrations map[string]Build) []Result {
	root := node
	if root.Kind == yaml.DocumentNode {
		root = root.Content[0]
	}

	var global, receivers *yaml.Node
	for _, kv := range pairs(root) {
		switch kv.key.Value {
		case "global":
			global = kv.value
		case "receivers":
			receivers = kv.value
		}
	}

	var results []Result
	for _, rec := range receivers.Content {
		var name string
		for _, kv := range pairs(rec) {
			if kv.key.Value == "name" {
				name = kv.value.Value
			}
		}

		for _, kv := range pairs(rec) {
			prefix, ok := strings.CutSuffix(kv.key.Value, "_configs")
			if !ok {
				continue
			}
			build, ok := integrations[prefix]
			if !ok {
				results = append(results, Result{
					Integration: Integration{Name: prefix, Receiver: name},
					Err:         fmt.Errorf("unknown integration %q", prefix),
				})
				continue
			}
			for idx, entry := range kv.value.Content {
				n, err := build(global, entry)
				results = append(results, Result{
					Integration: Integration{Name: prefix, Receiver: name, Index: idx, Notifier: n},
					Err:         err,
				})
			}
		}
	}
	return results
}

// Globals is the global half of the config file. An integration pulls its own
// keys out by name, so the key a global lives under is written down once, in
// the integration, and nothing derives it.
type Globals struct {
	node *yaml.Node
	errs *Errors
}

// Get reads one global key into the config field it belongs to. The type comes
// from the field, so it is written down once.
func Get[T any](g *Globals, key string, into **T) {
	for _, kv := range pairs(g.node) {
		if kv.key.Value != key {
			continue
		}
		var v T
		if err := kv.value.Decode(&v); err != nil {
			g.errs.Addf("%s: %v", key, err)
			return
		}
		*into = &v
	}
}

// Merger merges the global half of a config into the receiver half. The groups
// are resolved to fields once, at registration.
type Merger[C any] struct {
	groups []string
}

func NewMerger[C any](groups [][]string) Merger[C] {
	var zero C
	t := reflect.TypeOf(zero)

	byKey := map[string]int{}
	for i := 0; i < t.NumField(); i++ {
		key, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		byKey[key] = i
	}

	m := Merger[C]{groups: make([]string, t.NumField())}
	for _, group := range groups {
		for _, key := range group {
			i, ok := byKey[key]
			if !ok {
				panic(fmt.Sprintf("library: %s has no field for group key %q", t.Name(), key))
			}
			m.groups[i] = group[0]
		}
	}
	return m
}

// Merge fills every field the receiver left unset from the global half. Fields
// sharing a group are inherited together or not at all, so a receiver naming
// its own api_url does not also inherit the global api_url_file.
func (m Merger[C]) Merge(global, receiver C) C {
	out := reflect.ValueOf(&receiver).Elem()
	from := reflect.ValueOf(global)

	claimed := map[string]bool{}
	for i, group := range m.groups {
		if group != "" && !out.Field(i).IsNil() {
			claimed[group] = true
		}
	}

	for i, group := range m.groups {
		if !out.Field(i).IsNil() || claimed[group] {
			continue
		}
		out.Field(i).Set(from.Field(i))
	}
	return receiver
}

// checkOptional fails at registration, which runs at init, rather than leaving
// a non optional field to panic on whichever config first reaches it.
func checkOptional(conf any) {
	t := reflect.TypeOf(conf)
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Type.Kind() != reflect.Ptr {
			panic(fmt.Sprintf("library: %s.%s must be a pointer, every config field is optional",
				t.Name(), t.Field(i).Name))
		}
	}
}

type Errors struct {
	errs []error
}

func (e *Errors) Addf(format string, args ...any) {
	e.errs = append(e.errs, fmt.Errorf(format, args...))
}

func (e *Errors) Add(err error) {
	if err != nil {
		e.errs = append(e.errs, err)
	}
}

func (e *Errors) Err() error {
	return errors.Join(e.errs...)
}

type pair struct{ key, value *yaml.Node }

func pairs(n *yaml.Node) []pair {
	var ps []pair
	if n == nil {
		return ps
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		ps = append(ps, pair{n.Content[i], n.Content[i+1]})
	}
	return ps
}

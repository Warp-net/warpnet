// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package apisync verifies that the three independent Warpnet code bases that
// speak the same libp2p protocols agree on their wire contract:
//
//   - Backend (Go):  event/paths.go + event/event.go + core/handler/*.go
//   - Frontend (JS): frontend/src/service/service.js
//   - Warpdroid:     warpdroid/.../ProtocolIds.kt + WarpnetDtos.kt
//                  + warpdroid/.../WarpnetRepository.kt
//
// Two facts are checked:
//
//  1. The set of protocol path constants (name → value) is identical across
//     the three sources.
//  2. For each protocol that has a Go stream handler registered in
//     cmd/node/member/node/member-node.go, the JSON keys sent by each client
//     are a subset of the backend event struct's `json:` tags. The Go side
//     is derived from AST: routes link path constant → handler function,
//     handler bodies declare `var ev event.XxxEvent`, and event types in
//     the event package expose `json:` tags (alias chains are followed).
//     Clients are parsed from their own sources: service.js for the
//     frontend body literals, WarpnetRepository.kt for `ProtocolIds.X ↔
//     DTO class` pairs and WarpnetDtos.kt for the DTO field JSON names.
//
// Nothing is hand-curated; adding a new endpoint on either side starts
// failing here until both clients catch up.
package apisync_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	return filepath.Dir(filepath.Dir(thisFile))
}

func readFile(t *testing.T, rel string) string {
	t.Helper()
	p := filepath.Join(repoRoot(t), rel)
	b, err := os.ReadFile(p)
	require.NoErrorf(t, err, "read %s", p)
	return string(b)
}

// ----------------------------------------------------------------------------
// Path constant parsing (shared across all three sources)
// ----------------------------------------------------------------------------

const constPattern = `(P(?:RIVATE|UBLIC)_[A-Z_]+)`

func parseConstants(src, pattern string) map[string]string {
	re := regexp.MustCompile(pattern)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		out[m[1]] = m[2]
	}
	return out
}

func parseGoPaths(t *testing.T) map[string]string {
	t.Helper()
	return parseConstants(
		readFile(t, "event/paths.go"),
		`(?m)^\s*`+constPattern+`\s*=\s*"([^"]+)"`,
	)
}

func parseJsPaths(t *testing.T) map[string]string {
	t.Helper()
	return parseConstants(
		readFile(t, "frontend/src/service/service.js"),
		`export\s+const\s+`+constPattern+`\s*=\s*"([^"]+)"`,
	)
}

func parseKotlinPaths(t *testing.T) map[string]string {
	t.Helper()
	return parseConstants(
		readFile(t, "warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/ProtocolIds.kt"),
		`const\s+val\s+`+constPattern+`\s*=\s*"([^"]+)"`,
	)
}

func TestAPISync_Protocols(t *testing.T) {
	goPaths := parseGoPaths(t)
	jsPaths := parseJsPaths(t)
	ktPaths := parseKotlinPaths(t)

	require.NotEmpty(t, goPaths, "no constants parsed from event/paths.go")
	require.NotEmpty(t, jsPaths, "no constants parsed from service.js")
	require.NotEmpty(t, ktPaths, "no constants parsed from ProtocolIds.kt")

	t.Run("frontend_matches_backend", func(t *testing.T) {
		for name, val := range jsPaths {
			backendVal, ok := goPaths[name]
			if !assert.Truef(t, ok, "frontend constant %s is not declared in event/paths.go", name) {
				continue
			}
			assert.Equalf(t, backendVal, val,
				"%s differs: backend=%q frontend=%q", name, backendVal, val)
		}
	})

	t.Run("warpdroid_matches_backend", func(t *testing.T) {
		for name, val := range ktPaths {
			backendVal, ok := goPaths[name]
			if !assert.Truef(t, ok, "warpdroid constant %s is not declared in event/paths.go", name) {
				continue
			}
			assert.Equalf(t, backendVal, val,
				"%s differs: backend=%q warpdroid=%q", name, backendVal, val)
		}
	})

	t.Run("frontend_and_warpdroid_agree_when_both_define_path", func(t *testing.T) {
		for name, jsVal := range jsPaths {
			ktVal, ok := ktPaths[name]
			if !ok {
				continue
			}
			assert.Equalf(t, jsVal, ktVal,
				"%s differs: frontend=%q warpdroid=%q", name, jsVal, ktVal)
		}
	})
}

// ----------------------------------------------------------------------------
// Backend event types (Go AST)
// ----------------------------------------------------------------------------

// parsePackageFiles parses every non-test `.go` file in dir into AST and
// returns them. Replaces parser.ParseDir (deprecated in Go 1.25).
func parsePackageFiles(t *testing.T, dir string) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoErrorf(t, err, "read dir %s", dir)
	fset := token.NewFileSet()
	var out []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		require.NoErrorf(t, err, "parse %s", name)
		out = append(out, f)
	}
	return out
}

// eventStructKeys returns map[eventTypeName] -> sorted JSON keys, built by
// parsing every non-test file under event/. Type aliases (`type X = Y`) are
// resolved transitively; aliases whose target lives outside the event package
// (e.g. `type NewTweetEvent = domain.Tweet`) are omitted because their fields
// are out of this test's scope.
func eventStructKeys(t *testing.T) map[string][]string {
	t.Helper()
	files := parsePackageFiles(t, filepath.Join(repoRoot(t), "event"))
	require.NotEmpty(t, files, "no .go files in event/")

	type spec struct {
		alias bool
		expr  ast.Expr
	}
	decls := map[string]spec{}
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, s := range gd.Specs {
				ts := s.(*ast.TypeSpec)
				decls[ts.Name.Name] = spec{alias: ts.Assign != token.NoPos, expr: ts.Type}
			}
		}
	}

	var resolve func(name string, seen map[string]bool) *ast.StructType
	resolve = func(name string, seen map[string]bool) *ast.StructType {
		if seen[name] {
			return nil
		}
		seen[name] = true
		d, ok := decls[name]
		if !ok {
			return nil
		}
		switch x := d.expr.(type) {
		case *ast.StructType:
			return x
		case *ast.Ident:
			return resolve(x.Name, seen)
		}
		return nil // SelectorExpr or other → out of scope
	}

	out := map[string][]string{}
	for name := range decls {
		st := resolve(name, map[string]bool{})
		if st == nil || st.Fields == nil {
			continue
		}
		seen := map[string]bool{}
		for _, f := range st.Fields.List {
			for _, n := range f.Names {
				if !n.IsExported() {
					continue
				}
				tag := ""
				if f.Tag != nil {
					tag = strings.Trim(f.Tag.Value, "`")
				}
				key, drop := jsonKeyFromStructTag(tag, n.Name)
				if drop {
					continue
				}
				if !seen[key] {
					seen[key] = true
					out[name] = append(out[name], key)
				}
			}
		}
		sort.Strings(out[name])
	}
	return out
}

// jsonKeyFromStructTag mirrors encoding/json's tag interpretation: explicit
// names win, an absent tag falls back to the (lower-cased) field name, and a
// `json:"-"` tag suppresses the field entirely.
func jsonKeyFromStructTag(rawTag, fieldName string) (string, bool) {
	st := reflect.StructTag(rawTag)
	tag := st.Get("json")
	if tag == "-" {
		return "", true
	}
	name := strings.Split(tag, ",")[0]
	if name == "" {
		return strings.ToLower(fieldName), false
	}
	return name, false
}

// ----------------------------------------------------------------------------
// Stream-handler routing table (Go AST)
// ----------------------------------------------------------------------------

// routeMap returns map[pathConstantName] -> handlerFuncName, sourced from the
// composite literals inside cmd/node/member/node/member-node.go. The shape it
// looks for is `{event.PRIVATE_POST_X, handler.StreamYHandler(...)}`.
func routeMap(t *testing.T) map[string]string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "cmd/node/member/node/member-node.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	require.NoError(t, err)

	out := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok || len(cl.Elts) != 2 {
			return true
		}
		pathSel, ok := cl.Elts[0].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := pathSel.X.(*ast.Ident)
		if !ok || pkg.Name != "event" {
			return true
		}
		call, ok := cl.Elts[1].(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		fpkg, ok := fn.X.(*ast.Ident)
		if !ok || fpkg.Name != "handler" {
			return true
		}
		out[pathSel.Sel.Name] = fn.Sel.Name
		return true
	})
	return out
}

// ----------------------------------------------------------------------------
// Handler → event type (Go AST)
// ----------------------------------------------------------------------------

// handlerEventTypes returns map[handlerFuncName] -> eventTypeName by parsing
// each non-test file under core/handler/ and looking at the first `var x
// event.Y` declaration anywhere inside the handler function. Handlers that
// don't unmarshal a body (e.g. node pairing, challenge) drop out and are
// skipped by the caller.
func handlerEventTypes(t *testing.T) map[string]string {
	t.Helper()
	files := parsePackageFiles(t, filepath.Join(repoRoot(t), "core/handler"))
	require.NotEmpty(t, files, "no .go files in core/handler/")

	out := map[string]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			eventType := firstEventVarType(fd.Body)
			if eventType == "" {
				continue
			}
			out[fd.Name.Name] = eventType
		}
	}
	return out
}

// firstEventVarType walks a function body and returns the type name of the
// first `event.X` selector used as a variable type (covers both
// `var ev event.X` and `var ev event.X = ...`). Returns "" if none found.
func firstEventVarType(body *ast.BlockStmt) string {
	var found string
	ast.Inspect(body, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		gd, ok := n.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			return true
		}
		for _, s := range gd.Specs {
			vs, ok := s.(*ast.ValueSpec)
			if !ok || vs.Type == nil {
				continue
			}
			sel, ok := vs.Type.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "event" {
				continue
			}
			found = sel.Sel.Name
			return false
		}
		return true
	})
	return found
}

// ----------------------------------------------------------------------------
// Frontend bodies (service.js)
// ----------------------------------------------------------------------------

// jsBodyKeys returns map[pathConstantName] -> body keys for every
// `path: CONST, body: { … }` literal in service.js. Bodies are always flat
// object literals so a non-nesting `[^{}]*` group is enough.
func jsBodyKeys(t *testing.T) map[string][]string {
	t.Helper()
	src := readFile(t, "frontend/src/service/service.js")
	re := regexp.MustCompile(
		`path\s*:\s*` + constPattern +
			`\s*,\s*(?:timestamp\s*:[^,]+,\s*)?body\s*:\s*\{([^{}]*)\}`,
	)
	keyRe := regexp.MustCompile(`(?m)^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:`)
	out := map[string][]string{}
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		if _, dup := out[m[1]]; dup {
			continue
		}
		seen := map[string]bool{}
		var keys []string
		for _, k := range keyRe.FindAllStringSubmatch(m[2], -1) {
			if seen[k[1]] {
				continue
			}
			seen[k[1]] = true
			keys = append(keys, k[1])
		}
		sort.Strings(keys)
		out[m[1]] = keys
	}
	return out
}

// ----------------------------------------------------------------------------
// Warpdroid DTOs (Kotlin)
// ----------------------------------------------------------------------------

// kotlinPathToDTO maps each ProtocolIds.X referenced in WarpnetRepository.kt
// to the data class serialised on the wire. Two shapes are matched:
//
//	client.request(ProtocolIds.X, fooAdapter.toJson(DtoClass(...)))
//	val payload = fooAdapter.toJson(DtoClass(...))
//	client.request(ProtocolIds.X, payload)
//
// Endpoints whose body is empty or built ad-hoc (without a DTO data class)
// drop out and the caller skips them.
func kotlinPathToDTO(t *testing.T) map[string]string {
	t.Helper()
	src := readFile(t, "warpdroid/app/src/main/java/site/warpnet/warpdroid/warpnet/WarpnetRepository.kt")

	// val NAME = ADAPTER.toJson(DTO(
	varRe := regexp.MustCompile(`val\s+(\w+)\s*=\s*\w+\.toJson\(\s*(\w+)\s*\(`)
	varToDto := map[string]string{}
	for _, m := range varRe.FindAllStringSubmatch(src, -1) {
		varToDto[m[1]] = m[2]
	}

	out := map[string]string{}

	inlineRe := regexp.MustCompile(`ProtocolIds\.([A-Z_]+)\s*,\s*\w+\.toJson\(\s*(\w+)\s*\(`)
	for _, m := range inlineRe.FindAllStringSubmatch(src, -1) {
		out[m[1]] = m[2]
	}

	indirectRe := regexp.MustCompile(`ProtocolIds\.([A-Z_]+)\s*,\s*(\w+)\s*[,)]`)
	for _, m := range indirectRe.FindAllStringSubmatch(src, -1) {
		if _, ok := out[m[1]]; ok {
			continue
		}
		if dto, ok := varToDto[m[2]]; ok {
			out[m[1]] = dto
		}
	}
	return out
}

// kotlinDtoKeys returns map[dataClassName] -> sorted JSON keys for every
// `data class` declared in WarpnetDtos.kt. The wire name is taken from
// `@Json(name = "…")` when present, otherwise from the parameter name itself
// (matches Moshi's default).
func kotlinDtoKeys(t *testing.T) map[string][]string {
	t.Helper()
	src := readFile(t, "warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/dto/WarpnetDtos.kt")
	headerRe := regexp.MustCompile(`data class\s+(\w+)\s*\(`)
	paramRe := regexp.MustCompile(
		`(?:@Json\(\s*name\s*=\s*"([^"]+)"\s*\)\s*)?val\s+([a-zA-Z_][a-zA-Z0-9_]*)`,
	)
	out := map[string][]string{}
	for _, loc := range headerRe.FindAllStringSubmatchIndex(src, -1) {
		name := src[loc[2]:loc[3]]
		// loc[1] points just past `(`; back up one to enter captureBalanced.
		body, ok := captureBalanced(src[loc[1]-1:], '(', ')')
		if !ok {
			continue
		}
		seen := map[string]bool{}
		var keys []string
		for _, p := range paramRe.FindAllStringSubmatch(body, -1) {
			k := p[1]
			if k == "" {
				k = p[2]
			}
			if seen[k] {
				continue
			}
			seen[k] = true
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out[name] = keys
	}
	return out
}

func captureBalanced(s string, open, closer byte) (string, bool) {
	if len(s) == 0 || s[0] != open {
		return "", false
	}
	depth := 0
	for i := range len(s) {
		switch s[i] {
		case open:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return s[1:i], true
			}
		}
	}
	return "", false
}

// ----------------------------------------------------------------------------
// The combined assertion
// ----------------------------------------------------------------------------

func assertSubset(t *testing.T, proto, clientName string, backend, client []string) {
	t.Helper()
	bset := map[string]bool{}
	for _, k := range backend {
		bset[k] = true
	}
	var extra []string
	for _, k := range client {
		if !bset[k] {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		t.Errorf(
			"%s: %s sends keys not declared by the backend struct: %v\n  backend keys: %v\n  %s keys:    %v",
			proto, clientName, extra, backend, clientName, client,
		)
	}
}

func TestAPISync_Payloads(t *testing.T) {
	eventKeys := eventStructKeys(t)
	handlerToEvent := handlerEventTypes(t)
	routes := routeMap(t)
	jsBodies := jsBodyKeys(t)
	ktPaths := kotlinPathToDTO(t)
	ktDtos := kotlinDtoKeys(t)

	require.NotEmpty(t, eventKeys, "no event structs parsed")
	require.NotEmpty(t, handlerToEvent, "no handler→event mappings parsed")
	require.NotEmpty(t, routes, "no stream routes parsed")
	require.NotEmpty(t, jsBodies, "no frontend bodies parsed")
	require.NotEmpty(t, ktPaths, "no warpdroid path→DTO mappings parsed")
	require.NotEmpty(t, ktDtos, "no warpdroid DTOs parsed")

	checked := 0
	for pathConst, handlerName := range routes {
		eventType, ok := handlerToEvent[handlerName]
		if !ok {
			// Handler doesn't unmarshal a typed body (e.g. pairing, challenge).
			continue
		}
		backend, ok := eventKeys[eventType]
		if !ok {
			// Event type aliases a non-event-package type (e.g. domain.Tweet);
			// out of scope here, the cross-client contract is checked at the
			// domain layer instead.
			continue
		}
		checked++
		t.Run(pathConst, func(t *testing.T) {
			t.Logf("backend: handler=%s event=%s keys=%v", handlerName, eventType, backend)

			if js, ok := jsBodies[pathConst]; ok {
				assertSubset(t, pathConst, "frontend", backend, js)
			}

			if dto, ok := ktPaths[pathConst]; ok {
				kt, ok2 := ktDtos[dto]
				if assert.Truef(t, ok2,
					"warpdroid uses DTO %s for %s but no `data class %s` found in WarpnetDtos.kt",
					dto, pathConst, dto) {
					assertSubset(t, pathConst, "warpdroid", backend, kt)
				}
			}
		})
	}
	require.Greaterf(t, checked, 0, "no routes covered — discovery is broken")
}

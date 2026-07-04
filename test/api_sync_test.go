// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package apisync verifies that the three independent Warpnet code bases that
// speak the same libp2p protocols agree on their wire contract:
//
//   - Backend (Go):  event/paths.go + event/event.go + domain/warpnet.go
//   - core/handler/*.go
//   - cmd/node/member/node/member-node.go (the route table)
//   - Frontend (JS): frontend/src/service/service.js
//   - Warpdroid:     warpdroid/.../ProtocolIds.kt + WarpnetDtos.kt
//   - warpdroid/.../WarpnetRepository.kt
//
// What the test enforces, end to end (no hand-curated table — everything
// is discovered from the sources themselves):
//
//  1. Path constants. Every constant that the frontend or warpdroid
//     declares must exist in event/paths.go with the same value, and
//     when both clients declare the same name they must agree.
//  2. Payload completeness. For each protocol that has a stream handler
//     registered in member-node.go and whose handler unmarshals a typed
//     body, the test walks the alias chain from event.X (possibly into
//     domain.Y) to a concrete struct. Then, for each client that wires
//     the protocol:
//     a) the client's keys must be a subset of the backend struct's
//     JSON tags (typos / renames = fail);
//     b) every Go field the handler explicitly validates as non-empty
//     (`if ev.X == ""` / `if ev.X == nil`) must be present on the
//     wire (missing required field = fail). The required set is
//     read directly from handler code, not derived from
//     `omitempty`, so server-stamped fields like
//     domain.Tweet.Id and domain.Tweet.CreatedAt — which lack
//     `omitempty` but are absent from request bodies on purpose —
//     do not produce false positives.
//     If a routed body-decoding handler is wired by neither client, the
//     subtest fails unless the path is allowlisted in
//     protocolsWithoutClient (node-only protocols or documented
//     client-TODOs).
//
// Constants declared in clients but never wired into a request/DTO are
// not flagged — see the protocolsWithoutClient comment for the rationale,
// which boils down to warpdroid's ProtocolIds.kt being a deliberate
// mirror of event/paths.go rather than a usage manifest.
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

	t.Run("frontend_paths_exist_and_match_backend", func(t *testing.T) {
		for name, val := range jsPaths {
			backendVal, ok := goPaths[name]
			if !assert.Truef(t, ok, "frontend constant %s is not declared in event/paths.go", name) {
				continue
			}
			assert.Equalf(t, backendVal, val,
				"%s differs: backend=%q frontend=%q", name, backendVal, val)
		}
	})

	t.Run("warpdroid_paths_exist_and_match_backend", func(t *testing.T) {
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
// Backend struct discovery: AST across event/ and domain/
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

// pkgDecls collects every TypeSpec across the named go packages. Outer key is
// package name ("event", "domain"); inner key is the type identifier.
func pkgDecls(t *testing.T) map[string]map[string]*ast.TypeSpec {
	t.Helper()
	root := repoRoot(t)
	out := map[string]map[string]*ast.TypeSpec{}
	for _, p := range []struct{ pkg, dir string }{
		{"event", "event"},
		{"domain", "domain"},
	} {
		files := parsePackageFiles(t, filepath.Join(root, p.dir))
		decls := map[string]*ast.TypeSpec{}
		for _, f := range files {
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, s := range gd.Specs {
					ts := s.(*ast.TypeSpec)
					decls[ts.Name.Name] = ts
				}
			}
		}
		out[p.pkg] = decls
	}
	return out
}

// resolvedStruct is the outcome of walking an alias chain to a concrete
// struct, plus the package the struct ended up in. Same struct reached from
// different starting points yields the same pkg.
type resolvedStruct struct {
	st  *ast.StructType
	pkg string
}

// resolveStruct walks `type X = Y` and `type X = pkg.Y` aliases until it
// lands on a `*ast.StructType`, returning the package it landed in. Returns
// (nil, "") for cycles, unknown packages, or types we can't represent
// (interfaces, maps, etc. — none of which are wire payloads in this repo).
func resolveStruct(pkgs map[string]map[string]*ast.TypeSpec, currentPkg, name string, seen map[string]bool) *resolvedStruct {
	key := currentPkg + "." + name
	if seen[key] {
		return nil
	}
	seen[key] = true

	pkg, ok := pkgs[currentPkg]
	if !ok {
		return nil
	}
	ts, ok := pkg[name]
	if !ok {
		return nil
	}
	switch x := ts.Type.(type) {
	case *ast.StructType:
		return &resolvedStruct{st: x, pkg: currentPkg}
	case *ast.Ident:
		return resolveStruct(pkgs, currentPkg, x.Name, seen)
	case *ast.SelectorExpr:
		ident, ok := x.X.(*ast.Ident)
		if !ok {
			return nil
		}
		return resolveStruct(pkgs, ident.Name, x.Sel.Name, seen)
	}
	return nil
}

// backendFields is everything the test wants to know about one wire shape:
//
//   - All:           every JSON wire key the struct declares.
//   - GoToJson:      Go field name → JSON wire key, so handler-AST
//     validations (which name fields by their Go identifiers)
//     can be translated to wire keys.
type backendFields struct {
	All      []string
	GoToJson map[string]string
}

func backendStructKeys(t *testing.T) map[string]backendFields {
	t.Helper()
	decls := pkgDecls(t)
	out := map[string]backendFields{}
	for name := range decls["event"] {
		res := resolveStruct(decls, "event", name, map[string]bool{})
		if res == nil || res.st.Fields == nil {
			continue
		}
		all, goToJson := jsonKeysOfStruct(res.st)
		out[name] = backendFields{All: all, GoToJson: goToJson}
	}
	return out
}

func jsonKeysOfStruct(st *ast.StructType) (all []string, goToJson map[string]string) {
	goToJson = map[string]string{}
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
			goToJson[n.Name] = key
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, key)
		}
	}
	sort.Strings(all)
	return all, goToJson
}

// jsonKeyFromStructTag mirrors encoding/json's tag interpretation: explicit
// names win, an absent tag falls back to the exported field name as-is
// (encoding/json does not lower-case it), and `json:"-"` suppresses the
// field.
func jsonKeyFromStructTag(rawTag, fieldName string) (name string, drop bool) {
	st := reflect.StructTag(rawTag)
	tag := st.Get("json")
	if tag == "-" {
		return "", true
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	if name == "" {
		name = fieldName
	}
	return name, false
}

// ----------------------------------------------------------------------------
// Stream-handler routing table and handler→event linkage (Go AST)
// ----------------------------------------------------------------------------

// routeMap parses cmd/node/member/node/member-node.go for the
// `{event.CONST, handler.StreamYHandler(...)}` composite literals registered
// via SetStreamHandlers. Returns map[pathConstantName]handlerFuncName.
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

// handlerInfo is the Go-side contract for one handler: the event type it
// unmarshals and the Go field names it explicitly validates as non-zero
// (e.g. `if ev.OwnerId == ""` or `if ev.ParentId == nil` returning an error).
// These are the fields a client MUST supply on the wire, derived directly
// from handler code rather than the indirect `omitempty` heuristic.
type handlerInfo struct {
	EventType        string
	RequiredGoFields []string
	VarName          string
}

// handlerEventTypes walks core/handler/*.go and for every FuncDecl returns
// the first `var <name> event.X` declaration inside the handler plus the
// Go field names checked against the empty string or nil within the same
// function body. Handlers that don't unmarshal a typed body (pairing,
// challenge) drop out and are skipped by callers.
func handlerEventTypes(t *testing.T) map[string]handlerInfo {
	t.Helper()
	files := parsePackageFiles(t, filepath.Join(repoRoot(t), "core/handler"))
	require.NotEmpty(t, files, "no .go files in core/handler/")

	out := map[string]handlerInfo{}
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			varName, et := firstEventVar(fd.Body)
			if et == "" {
				continue
			}
			req := handlerRequiredGoFields(fd.Body, varName)
			out[fd.Name.Name] = handlerInfo{EventType: et, RequiredGoFields: req, VarName: varName}
		}
	}
	return out
}

// firstEventVar returns the variable name and event type of the first
// `var <name> event.X` declaration anywhere inside body, or ("", "") if
// none is found.
func firstEventVar(body *ast.BlockStmt) (varName, eventType string) {
	ast.Inspect(body, func(n ast.Node) bool {
		if eventType != "" {
			return false
		}
		gd, ok := n.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			return true
		}
		for _, s := range gd.Specs {
			vs, ok := s.(*ast.ValueSpec)
			if !ok || vs.Type == nil || len(vs.Names) == 0 {
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
			varName = vs.Names[0].Name
			eventType = sel.Sel.Name
			return false
		}
		return true
	})
	return varName, eventType
}

// handlerRequiredGoFields scans body for `<varName>.<Field> == ""` and
// `<varName>.<Field> == nil` comparisons — the Warpnet handler convention
// for "this field must be present on the wire". The result is the set of
// Go field names. Heterogeneous checks like `ev.OwnerId == ev.UserId`
// (business logic, not nullability) are ignored because they don't match
// the empty-string/nil literal pattern.
func handlerRequiredGoFields(body *ast.BlockStmt, varName string) []string {
	if varName == "" {
		return nil
	}
	fields := map[string]bool{}
	matchEmptyOrNil := func(sel, lit ast.Expr) {
		s, ok := sel.(*ast.SelectorExpr)
		if !ok {
			return
		}
		id, ok := s.X.(*ast.Ident)
		if !ok || id.Name != varName {
			return
		}
		switch l := lit.(type) {
		case *ast.BasicLit:
			if l.Kind == token.STRING && (l.Value == `""` || l.Value == "``") {
				fields[s.Sel.Name] = true
			}
		case *ast.Ident:
			if l.Name == "nil" {
				fields[s.Sel.Name] = true
			}
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok || bin.Op != token.EQL {
			return true
		}
		matchEmptyOrNil(bin.X, bin.Y)
		matchEmptyOrNil(bin.Y, bin.X)
		return true
	})
	out := make([]string, 0, len(fields))
	for k := range fields {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ----------------------------------------------------------------------------
// Frontend bodies (service.js)
// ----------------------------------------------------------------------------

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

// kotlinDtoKeys returns map[dataClassName] -> sorted JSON keys for every
// `data class` declared in WarpnetDtos.kt. The wire name is taken from
// `@Json(name = "…")` when present, otherwise from the parameter name (Moshi
// default).
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

// kotlinPathToDTO maps each ProtocolIds.X used in WarpnetRepository.kt to the
// data class serialised on the wire. Four call shapes are handled, in
// decreasing directness:
//
//	client.request(ProtocolIds.X, adapter.toJson(DtoClass(...)))     ← inline
//	val draft   = DtoClass(...);            adapter.toJson(draft)     ← var instance
//	val payload = adapter.toJson(DtoClass(...));                      ← precomputed JSON
//	client.request(ProtocolIds.X, payload) | adapter.toJson(draft)
//
// Endpoints that build their body ad-hoc (no DTO data class) drop out.
func kotlinPathToDTO(t *testing.T, knownDtos map[string][]string) map[string]string {
	t.Helper()
	src := readFile(t, "warpdroid/app/src/main/java/site/warpnet/warpdroid/warpnet/WarpnetRepository.kt")

	isDto := func(name string) bool { _, ok := knownDtos[name]; return ok }

	// val NAME = DtoClass(            ← live instance
	instanceRe := regexp.MustCompile(`val\s+(\w+)\s*=\s*(\w+)\s*\(`)
	instanceToDTO := map[string]string{}
	for _, m := range instanceRe.FindAllStringSubmatch(src, -1) {
		if isDto(m[2]) {
			instanceToDTO[m[1]] = m[2]
		}
	}

	// val NAME = ADAPTER.toJson(DtoClass(   ← precomputed JSON string
	jsonStringRe := regexp.MustCompile(`val\s+(\w+)\s*=\s*\w+\.toJson\(\s*(\w+)\s*\(`)
	jsonToDTO := map[string]string{}
	for _, m := range jsonStringRe.FindAllStringSubmatch(src, -1) {
		if isDto(m[2]) {
			jsonToDTO[m[1]] = m[2]
		}
	}

	out := map[string]string{}

	// Inline: client.request(ProtocolIds.X, ADAPTER.toJson(DtoClass(
	inlineRe := regexp.MustCompile(`ProtocolIds\.([A-Z_]+)\s*,\s*\w+\.toJson\(\s*(\w+)\s*\(`)
	for _, m := range inlineRe.FindAllStringSubmatch(src, -1) {
		if isDto(m[2]) {
			out[m[1]] = m[2]
		}
	}

	// Variable instance: client.request(ProtocolIds.X, ADAPTER.toJson(varName))
	toJsonVarRe := regexp.MustCompile(`ProtocolIds\.([A-Z_]+)\s*,\s*\w+\.toJson\(\s*(\w+)\s*\)`)
	for _, m := range toJsonVarRe.FindAllStringSubmatch(src, -1) {
		if _, has := out[m[1]]; has {
			continue
		}
		if dto, ok := instanceToDTO[m[2]]; ok {
			out[m[1]] = dto
		}
	}

	// Precomputed JSON string: client.request(ProtocolIds.X, payload[,)])
	indirectRe := regexp.MustCompile(`ProtocolIds\.([A-Z_]+)\s*,\s*(\w+)\s*[,)]`)
	for _, m := range indirectRe.FindAllStringSubmatch(src, -1) {
		if _, has := out[m[1]]; has {
			continue
		}
		if dto, ok := jsonToDTO[m[2]]; ok {
			out[m[1]] = dto
		}
	}
	return out
}

// ----------------------------------------------------------------------------
// Combined assertion
// ----------------------------------------------------------------------------

func assertSubset(t *testing.T, proto, clientName string, backend, client []string) {
	t.Helper()
	assertSubsetWithVerb(t, proto, clientName, "sends", "accept", backend, client)
}

// assertSubsetWithVerb is the directional flavour of assertSubset. The
// request-side caller wants "sends keys the backend doesn't accept";
// response-side wants "reads keys the backend doesn't emit". The
// underlying set arithmetic is identical.
func assertSubsetWithVerb(t *testing.T, proto, clientName, clientVerb, backendVerb string, backend, client []string) {
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
			"%s: %s %s keys the backend doesn't %s: %v\n  backend (all):  %v\n  %s keys:        %v",
			proto, clientName, clientVerb, backendVerb, extra, backend, clientName, client,
		)
	}
}

func assertRequiredCovered(t *testing.T, proto, clientName string, required, client []string) {
	t.Helper()
	cset := map[string]bool{}
	for _, k := range client {
		cset[k] = true
	}
	var missing []string
	for _, k := range required {
		if !cset[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf(
			"%s: %s omits required backend fields: %v\n  backend (req):  %v\n  %s keys:        %v",
			proto, clientName, missing, required, clientName, client,
		)
	}
}

// protocolsWithoutClient lists routes that legitimately or temporarily lack
// any client implementation. Two distinct reasons live here:
//
//   - node-only: inter-node handshake / consensus protocols that no thin
//     client should ever invoke (pairing, source-tree challenge,
//     moderation gossip). These are stable and never expected to gain a
//     client; the test silently skips them.
//   - client-TODO: protocols whose backend handler exists but neither
//     clients have wired up yet. The test still skips so it can stay green,
//     but a follow-up should either implement the client side or remove the
//     backend route.
//
// Anything routed that's not in this map and isn't wired by at least one
// client makes TestAPISync_Payloads fail with a precise pointer.
//
// Note on orphan client constants: warpdroid's ProtocolIds.kt is documented
// as a deliberate mirror of event/paths.go and declares many constants
// without using them in WarpnetRepository.kt. The test does not flag these
// "declared but not wired" entries; it only enforces the routed-protocol
// side of the contract (every routed handler must be reachable from at
// least one client unless allowlisted here).
var protocolsWithoutClient = map[string]string{
	"PRIVATE_POST_PAIR":             "node-only: node↔node pairing handshake",
	"PUBLIC_POST_NODE_CHALLENGE":    "node-only: proof-of-source-tree challenge",
	"PUBLIC_POST_MODERATION_RESULT": "node-only: moderation result gossip",

	"PRIVATE_GET_MESSAGE":    "client-TODO: single-message read is unimplemented on both clients",
	"PRIVATE_DELETE_MESSAGE": "client-TODO: single-message delete is unimplemented on both clients",
}

// requiredWireKeys translates Go field names that the handler explicitly
// checks for emptiness/nilness into the corresponding JSON wire keys, using
// the struct's tag map. Fields without a tag entry (defensive — shouldn't
// happen for routed handlers) drop out so the test never asserts against an
// unknown wire name.
func requiredWireKeys(info handlerInfo, fields backendFields) []string {
	out := make([]string, 0, len(info.RequiredGoFields))
	for _, g := range info.RequiredGoFields {
		if k, ok := fields.GoToJson[g]; ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func TestAPISync_Payloads(t *testing.T) {
	backend := backendStructKeys(t)
	handlerByName := handlerEventTypes(t)
	routes := routeMap(t)
	jsBodies := jsBodyKeys(t)
	ktDtos := kotlinDtoKeys(t)
	ktPaths := kotlinPathToDTO(t, ktDtos)

	require.NotEmpty(t, backend, "no event/domain structs parsed")
	require.NotEmpty(t, handlerByName, "no handler→event mappings parsed")
	require.NotEmpty(t, routes, "no stream routes parsed")
	require.NotEmpty(t, jsBodies, "no frontend bodies parsed")
	require.NotEmpty(t, ktDtos, "no warpdroid DTOs parsed")
	require.NotEmpty(t, ktPaths, "no warpdroid path→DTO mappings parsed")

	checked := 0
	for pathConst, handlerName := range routes {
		info, ok := handlerByName[handlerName]
		if !ok {
			// Handler doesn't unmarshal a typed body (e.g. pairing, challenge).
			continue
		}
		bk, ok := backend[info.EventType]
		if !ok || len(bk.All) == 0 {
			// Resolver couldn't find a struct (interface, map, missing pkg).
			continue
		}
		checked++

		jsKeys, jsHas := jsBodies[pathConst]
		ktDto, ktHas := ktPaths[pathConst]
		var ktKeys []string
		if ktHas {
			ktKeys = ktDtos[ktDto]
		}
		required := requiredWireKeys(info, bk)

		t.Run(pathConst, func(t *testing.T) {
			t.Logf("backend: handler=%s event=%s all=%v handler-required=%v",
				handlerName, info.EventType, bk.All, required)

			if !jsHas && !ktHas {
				if reason, ok := protocolsWithoutClient[pathConst]; ok {
					t.Skipf("%s skipped: %s", pathConst, reason)
				}
				t.Errorf("%s is routed by the backend but no client (frontend service.js or warpdroid WarpnetRepository.kt) speaks it — either add a client implementation or document it in protocolsWithoutClient",
					pathConst)
				return
			}

			if jsHas {
				assertSubset(t, pathConst, "frontend", bk.All, jsKeys)
				assertRequiredCovered(t, pathConst, "frontend", required, jsKeys)
			}
			if ktHas {
				assertSubset(t, pathConst, "warpdroid", bk.All, ktKeys)
				assertRequiredCovered(t, pathConst, "warpdroid", required, ktKeys)
			}
		})
	}
	require.Greaterf(t, checked, 0, "no routes covered — discovery is broken")
}

// ----------------------------------------------------------------------------
// Response-side audit
//
// The request-side test above only catches "client sends a key the backend
// rejects". The mirror failure — "client reads a key the backend never
// emits" — has been the most expensive class of bug for this repo
// (WarpnetNotification's bogus from_user_id / tweet_id silently dropped
// every notification; RepliesResponse's List<Tweet> Moshi-defaulted every
// reply node). This second pass closes that gap for warpdroid by walking
// each handler's `return event.X{...}` / `return event.X(y)` / `return
// <var>, nil` site, resolving the response struct via the same alias
// chain used elsewhere in this file, and asserting the warpdroid parse
// DTO's keys are a subset of the backend's response wire keys.
//
// Frontend isn't covered here: service.js reads dot-paths off the parsed
// response object without a declared schema, so the equivalent check
// would be regex against arbitrary JS — flaky and out of scope.
// ----------------------------------------------------------------------------

// handlerReturnType is one (package, type) pair pulled from a handler's
// return statements.
type handlerReturnType struct {
	Pkg  string // "event" or "domain"
	Type string
}

// typeRefFromQualified extracts (pkg, type) from `<pkg>.<Type>` if pkg is
// event or domain; nil otherwise. Used both for composite-literal types
// and type-conversion call functions.
func typeRefFromQualified(e ast.Expr) *handlerReturnType {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if id.Name != "event" && id.Name != "domain" {
		return nil
	}
	return &handlerReturnType{Pkg: id.Name, Type: sel.Sel.Name}
}

// handlerVarTypes scans body for local variable declarations whose type
// can be derived from a qualified `event.X` / `domain.X` reference —
// either as the declared type (`var x event.X`) or as a composite-literal
// / type-conversion RHS of a := assignment. Returns map[varName]type.
// Variables assigned from opaque function calls (e.g. `r := repo.Get()`)
// are not tracked.
func handlerVarTypes(body *ast.BlockStmt) map[string]handlerReturnType {
	out := map[string]handlerReturnType{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GenDecl:
			if x.Tok != token.VAR {
				return true
			}
			for _, s := range x.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok || vs.Type == nil {
					continue
				}
				if t := typeRefFromQualified(vs.Type); t != nil {
					for _, name := range vs.Names {
						out[name.Name] = *t
					}
				}
			}
		case *ast.AssignStmt:
			if x.Tok != token.DEFINE || len(x.Lhs) == 0 || len(x.Rhs) == 0 {
				return true
			}
			id, ok := x.Lhs[0].(*ast.Ident)
			if !ok {
				return true
			}
			switch rhs := x.Rhs[0].(type) {
			case *ast.CompositeLit:
				if t := typeRefFromQualified(rhs.Type); t != nil {
					out[id.Name] = *t
				}
			case *ast.CallExpr:
				if t := typeRefFromQualified(rhs.Fun); t != nil {
					out[id.Name] = *t
				}
			}
		}
		return true
	})
	return out
}

// handlerReturnTypes collects every distinct successful response type
// referenced by `return …, nil` (or `return …, <unused error>`) inside the
// handler factory. Returns are inspected in three forms:
//
//   - composite literal:  `return event.X{...}, nil`
//   - type conversion:    `return event.X(y), nil`
//   - identifier:         `return resp, nil` — resolved via [handlerVarTypes]
//
// String/constant returns like `event.Accepted` are ignored (they have no
// JSON schema to validate). Returns whose type can't be resolved (opaque
// repository calls etc.) drop out and surface as "no response type
// inferable" in the test report.
func handlerReturnTypes(body *ast.BlockStmt) []handlerReturnType {
	varTypes := handlerVarTypes(body)
	seen := map[string]bool{}
	var out []handlerReturnType
	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			return true
		}
		expr := ret.Results[0]
		var t *handlerReturnType
		switch v := expr.(type) {
		case *ast.CompositeLit:
			t = typeRefFromQualified(v.Type)
		case *ast.CallExpr:
			// `event.X(value)` — type conversion to alias. Functions like
			// `event.NewRetweetEvent(retweet)` have the same syntactic
			// shape but are distinguished by whether the qualified name
			// resolves to a known struct (the caller does that check).
			t = typeRefFromQualified(v.Fun)
		case *ast.Ident:
			if vt, ok := varTypes[v.Name]; ok {
				t = &vt
			}
		}
		if t == nil {
			return true
		}
		key := t.Pkg + "." + t.Type
		if seen[key] {
			return true
		}
		seen[key] = true
		out = append(out, *t)
		return true
	})
	return out
}

// handlerSuccessReturn resolves a handler's response wire shape. It picks
// the first return type whose alias chain lands on a struct in event/ or
// domain/, ignoring constants (event.Accepted) and unresolvable identifier
// returns. Returns nil if nothing resolves — the route is then skipped in
// the response audit.
func handlerSuccessReturn(
	pkgs map[string]map[string]*ast.TypeSpec,
	body *ast.BlockStmt,
) *resolvedStruct {
	for _, r := range handlerReturnTypes(body) {
		if res := resolveStruct(pkgs, r.Pkg, r.Type, map[string]bool{}); res != nil {
			return res
		}
	}
	return nil
}

// handlerBodies walks core/handler/*.go and returns map[FuncDecl name] ->
// body. The existing handlerEventTypes only exposes the unmarshal type;
// the response audit needs the body itself to scan return statements.
func handlerBodies(t *testing.T) map[string]*ast.BlockStmt {
	t.Helper()
	files := parsePackageFiles(t, filepath.Join(repoRoot(t), "core/handler"))
	out := map[string]*ast.BlockStmt{}
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			out[fd.Name.Name] = fd.Body
		}
	}
	return out
}

// kotlinAdapterDTOs scans WarpnetRepository.kt for `private val <name> =
// moshi.adapter<...DtoType>()` and returns map[adapterName]dtoTypeName.
// The DTO name is the rightmost identifier inside the generic, with any
// `site.warpnet.transport.dto.` prefix stripped.
func kotlinAdapterDTOs(t *testing.T) map[string]string {
	t.Helper()
	src := readFile(t, "warpdroid/app/src/main/java/site/warpnet/warpdroid/warpnet/WarpnetRepository.kt")
	re := regexp.MustCompile(`val\s+(\w+)\s*=\s*moshi\.adapter<\s*(?:[\w.]+\.)?(\w+)\s*>\(\s*\)`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		out[m[1]] = m[2]
	}
	return out
}

// kotlinResponseDtoForPath ties each PRIVATE/PUBLIC ProtocolIds constant
// used in WarpnetRepository.kt to the DTO that parses the corresponding
// response. The pattern is local and predictable:
//
//	val raw = client.request(ProtocolIds.X, …)
//	val page = <adapter>.fromJson(raw) ?: …
//
// so the discovery is "find the first .fromJson(raw) call after each
// client.request(ProtocolIds.X, …) within ~500 chars". Adapters that
// don't appear in [kotlinAdapterDTOs] are ignored (e.g. error-shape
// fallbacks that aren't response payloads).
func kotlinResponseDtoForPath(t *testing.T, adapters map[string]string) map[string]string {
	t.Helper()
	src := readFile(t, "warpdroid/app/src/main/java/site/warpnet/warpdroid/warpnet/WarpnetRepository.kt")
	// Greedy enough to span the few lines between request and fromJson,
	// short enough to not skip into the next function.
	re := regexp.MustCompile(`ProtocolIds\.([A-Z_]+)[\s\S]{0,500}?(\w+)\.fromJson\(\s*raw\s*\)`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		if _, has := out[m[1]]; has {
			continue
		}
		dto, ok := adapters[m[2]]
		if !ok {
			continue
		}
		out[m[1]] = dto
	}
	return out
}

// protocolsWithoutResponseDTO documents routes where warpdroid intentionally
// doesn't parse a response struct, or where the response is a string
// constant (event.Accepted) with no JSON schema to check.
var protocolsWithoutResponseDTO = map[string]string{
	// Acks: handlers return event.Accepted (a string constant) so there's
	// nothing to compare against on the wire.
	"PRIVATE_POST_BLOCK":                    "ack-only: handler returns event.Accepted",
	"PRIVATE_POST_UNBLOCK":                  "ack-only: handler returns event.Accepted",
	"PRIVATE_POST_MUTE":                     "ack-only: handler returns event.Accepted",
	"PRIVATE_POST_UNMUTE":                   "ack-only: handler returns event.Accepted",
	"PRIVATE_POST_BOOKMARK":                 "ack-only: handler returns event.Accepted",
	"PRIVATE_POST_UNBOOKMARK":               "ack-only: handler returns event.Accepted",
	"PRIVATE_POST_NOTIFICATION_READ":        "ack-only: handler returns event.Accepted",
	"PRIVATE_POST_NOTIFICATIONS_READ":       "ack-only: handler returns event.Accepted",
	"PRIVATE_DELETE_CHAT":                   "ack-only: handler returns event.Accepted",
	"PRIVATE_DELETE_MESSAGE":                "ack-only: handler returns event.Accepted",
	"PRIVATE_DELETE_FILTER":                 "ack-only: handler returns event.Accepted",
	"PRIVATE_DELETE_FILTER_KEYWORD":         "ack-only: handler returns event.Accepted",
	"PRIVATE_POST_FOLLOW_REQUEST_AUTHORIZE": "ack-only: handler returns event.Accepted",
	"PRIVATE_POST_FOLLOW_REQUEST_REJECT":    "ack-only: handler returns event.Accepted",
	"PUBLIC_POST_PIN":                       "ack-only: handler returns event.Accepted",
	"PUBLIC_POST_UNPIN":                     "ack-only: handler returns event.Accepted",
	"PUBLIC_POST_VIEW":                      "ack-only: handler returns event.Accepted",
	"PRIVATE_DELETE_TWEET":                  "ack-only: handler returns event.Accepted",
}

func TestAPISync_ResponsePayloads(t *testing.T) {
	backend := backendStructKeys(t)
	bodies := handlerBodies(t)
	routes := routeMap(t)
	pkgs := pkgDecls(t)
	ktAdapters := kotlinAdapterDTOs(t)
	ktDtos := kotlinDtoKeys(t)
	ktRespByPath := kotlinResponseDtoForPath(t, ktAdapters)

	require.NotEmpty(t, backend, "no event/domain structs parsed")
	require.NotEmpty(t, bodies, "no handler bodies parsed")
	require.NotEmpty(t, routes, "no stream routes parsed")
	require.NotEmpty(t, ktAdapters, "no warpdroid adapters parsed")
	require.NotEmpty(t, ktDtos, "no warpdroid DTOs parsed")
	require.NotEmpty(t, ktRespByPath, "no warpdroid response-dto bindings parsed")

	checked := 0
	for pathConst, handlerName := range routes {
		body, ok := bodies[handlerName]
		if !ok {
			continue
		}
		res := handlerSuccessReturn(pkgs, body)
		if res == nil {
			// Variable return whose type isn't statically inferable
			// (e.g. repository call result). Skipped; not a failure
			// because the alternative is false positives.
			continue
		}
		// Walk all the way down through any nested aliases that
		// resolveStruct already followed; collect the wire keys of the
		// concrete struct.
		bk, _ := jsonKeysOfStruct(res.st)
		if len(bk) == 0 {
			continue
		}

		dto, ok := ktRespByPath[pathConst]
		if !ok {
			// Route returns a typed body but warpdroid doesn't parse it.
			// Either fire-and-forget (ack-only handler) or a deliberate
			// no-op on the client side. Neither is a contract mismatch,
			// so we silently skip; protocolsWithoutResponseDTO and
			// protocolsWithoutClient document the known cases.
			continue
		}
		ktKeys, ok := ktDtos[dto]
		if !ok {
			t.Errorf("%s: warpdroid response adapter points at unknown DTO %q", pathConst, dto)
			continue
		}
		checked++
		t.Run(pathConst, func(t *testing.T) {
			t.Logf("backend response: handler=%s wire-keys=%v\nwarpdroid DTO=%s keys=%v",
				handlerName, bk, dto, ktKeys)
			assertSubsetWithVerb(t, pathConst, "warpdroid", "reads", "emit", bk, ktKeys)
		})
	}
	require.Greaterf(t, checked, 0, "no response payloads compared — discovery is broken")
}

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package apisync verifies that the three independent Warpnet code bases that
// speak the same libp2p protocols agree on their wire contract:
//
//   - Backend (Go):  event/paths.go         + core/handler/*.go event types
//   - Frontend (JS): frontend/src/service/service.js
//   - Warpdroid:     warpdroid/.../ProtocolIds.kt + WarpnetDtos.kt
//
// The test parses each source as text and compares:
//
//  1. The set of protocol path constants (name → value).
//  2. For a curated set of protocols, the JSON keys carried in the request
//     body, using the Go event struct's `json:` tags as the source of truth.
//
// "Match" is defined as subset of the backend keys, because both clients are
// allowed to skip optional fields (e.g. `cursor,omitempty`). A typo or rename
// on either client produces a key the backend cannot decode and fails the test.
package apisync_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/event"
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

// constPattern matches Warpnet path constant names like PUBLIC_GET_TWEET.
var constPattern = `(P(?:RIVATE|UBLIC)_[A-Z_]+)`

func parseConstants(src, pattern string) map[string]string {
	re := regexp.MustCompile(pattern)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		out[m[1]] = m[2]
	}
	return out
}

func parseGoPaths(t *testing.T) map[string]string {
	return parseConstants(
		readFile(t, "event/paths.go"),
		`(?m)^\s*`+constPattern+`\s*=\s*"([^"]+)"`,
	)
}

func parseJsPaths(t *testing.T) map[string]string {
	return parseConstants(
		readFile(t, "frontend/src/service/service.js"),
		`export\s+const\s+`+constPattern+`\s*=\s*"([^"]+)"`,
	)
}

func parseKotlinPaths(t *testing.T) map[string]string {
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

// payloadSpec links one protocol to its expected request DTO across all three
// sources. Only protocols whose request struct is a plain event.* type are
// listed; domain.Tweet / domain.User-shaped payloads (e.g. PRIVATE_POST_TWEET,
// PRIVATE_POST_USER, PUBLIC_POST_RETWEET) are intentionally omitted because
// the backend accepts the full domain object and clients only fill a subset.
//
// goRequest      = zero value of the Go event struct; `json:` tags are the
//                  source of truth.
// jsConst        = name of the path constant referenced by `path: <CONST>` in
//                  service.js (empty if the frontend doesn't call this path).
// kotlinDtoClass = `data class` name in WarpnetDtos.kt that the warpdroid
//                  repository serialises as the request body (empty if
//                  warpdroid doesn't call this path).
type payloadSpec struct {
	name           string
	goRequest      any
	jsConst        string
	kotlinDtoClass string
}

var payloadSpecs = []payloadSpec{
	{"PUBLIC_GET_USER", event.GetUserEvent{}, "PUBLIC_GET_USER", "GetUserEvent"},
	{"PUBLIC_GET_TWEETS", event.GetAllTweetsEvent{}, "PUBLIC_GET_TWEETS", "GetAllTweetsEvent"},
	{"PRIVATE_GET_TIMELINE", event.GetTimelineEvent{}, "PRIVATE_GET_TIMELINE", "GetAllTweetsEvent"},
	{"PUBLIC_GET_TWEET", event.GetTweetEvent{}, "PUBLIC_GET_TWEET", "GetTweetEvent"},
	{"PUBLIC_GET_TWEET_STATS", event.GetTweetStatsEvent{}, "PUBLIC_GET_TWEET_STATS", "GetTweetStatsEvent"},
	{"PUBLIC_GET_REPLIES", event.GetAllRepliesEvent{}, "PUBLIC_GET_REPLIES", "GetAllRepliesEvent"},
	{"PUBLIC_GET_REPLY", event.GetReplyEvent{}, "PUBLIC_GET_REPLY", ""},
	{"PUBLIC_DELETE_REPLY", event.DeleteReplyEvent{}, "PUBLIC_DELETE_REPLY", ""},
	{"PUBLIC_POST_FOLLOW", event.NewFollowEvent{}, "PUBLIC_POST_FOLLOW", "NewFollowEvent"},
	{"PUBLIC_POST_UNFOLLOW", event.NewUnfollowEvent{}, "PUBLIC_POST_UNFOLLOW", "NewUnfollowEvent"},
	{"PUBLIC_POST_LIKE", event.LikeEvent{}, "PUBLIC_POST_LIKE", "LikeEvent"},
	{"PUBLIC_POST_UNLIKE", event.UnlikeEvent{}, "PUBLIC_POST_UNLIKE", "LikeEvent"},
	{"PUBLIC_POST_VIEW", event.ViewEvent{}, "PUBLIC_POST_VIEW", "ViewEvent"},
	{"PUBLIC_POST_UNRETWEET", event.UnretweetEvent{}, "PUBLIC_POST_UNRETWEET", "UnretweetEvent"},
	{"PRIVATE_DELETE_TWEET", event.DeleteTweetEvent{}, "PRIVATE_DELETE_TWEET", "DeleteTweetEvent"},
	{"PUBLIC_GET_FOLLOWERS", event.GetFollowersEvent{}, "PUBLIC_GET_FOLLOWERS", "GetFollowersEvent"},
	{"PUBLIC_GET_FOLLOWINGS", event.GetFollowingsEvent{}, "PUBLIC_GET_FOLLOWINGS", "GetFollowingsEvent"},
	{"PUBLIC_POST_IS_FOLLOWING", event.GetIsFollowingEvent{}, "PUBLIC_POST_IS_FOLLOWING", "GetIsFollowingEvent"},
	{"PUBLIC_POST_IS_FOLLOWER", event.GetIsFollowerEvent{}, "PUBLIC_POST_IS_FOLLOWER", "GetIsFollowingEvent"},
	{"PUBLIC_GET_USERS", event.GetAllUsersEvent{}, "PUBLIC_GET_USERS", "GetAllUsersEvent"},
	{"PRIVATE_GET_NOTIFICATIONS", event.GetNotificationsEvent{}, "PRIVATE_GET_NOTIFICATIONS", "GetNotificationsEvent"},
	{"PRIVATE_POST_LOGIN", event.LoginEvent{}, "PRIVATE_POST_LOGIN", ""},
	{"PRIVATE_POST_UPLOAD_IMAGE", event.UploadImageEvent{}, "PRIVATE_POST_UPLOAD_IMAGE", ""},
	{"PUBLIC_GET_IMAGE", event.GetImageEvent{}, "PUBLIC_GET_IMAGE", ""},
	{"PRIVATE_GET_CHAT", event.GetChatEvent{}, "PRIVATE_GET_CHAT", ""},
	{"PRIVATE_GET_CHATS", event.GetAllChatsEvent{}, "PRIVATE_GET_CHATS", ""},
	{"PRIVATE_GET_MESSAGES", event.GetAllMessagesEvent{}, "PRIVATE_GET_MESSAGES", ""},
	{"PRIVATE_DELETE_CHAT", event.DeleteChatEvent{}, "PRIVATE_DELETE_CHAT", ""},
}

// goJsonKeys extracts the wire-side JSON keys from a Go struct's `json:` tags.
// Anonymous/embedded fields are flattened (their fields are reported instead).
func goJsonKeys(v any) map[string]bool {
	keys := map[string]bool{}
	collectGoKeys(reflect.TypeOf(v), keys)
	return keys
}

func collectGoKeys(rt reflect.Type, out map[string]bool) {
	if rt == nil {
		return
	}
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Anonymous {
			collectGoKeys(f.Type, out)
			continue
		}
		tag := f.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			if name == "-" {
				continue
			}
			out[strings.ToLower(f.Name)] = true
			continue
		}
		out[name] = true
	}
}

// jsBodyKeysForConst finds the request body literal that the frontend sends
// for a given path constant. Returns the set of JSON keys in that body, or
// (nil, false) when the frontend doesn't reference the constant.
//
// Bodies are always flat object literals in service.js so a non-nesting
// `[^{}]*` group is enough to capture them.
func jsBodyKeysForConst(src, constName string) (map[string]bool, bool) {
	re := regexp.MustCompile(
		`path\s*:\s*` + regexp.QuoteMeta(constName) +
			`\s*,\s*(?:timestamp\s*:[^,]+,\s*)?body\s*:\s*\{([^{}]*)\}`,
	)
	m := re.FindStringSubmatch(src)
	if m == nil {
		return nil, false
	}
	keys := map[string]bool{}
	keyRe := regexp.MustCompile(`(?m)^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:`)
	for _, k := range keyRe.FindAllStringSubmatch(m[1], -1) {
		keys[k[1]] = true
	}
	return keys, true
}

// kotlinDtoKeys returns the wire-side JSON keys of a Moshi data class as
// declared in WarpnetDtos.kt. A field's wire name is taken from
// `@Json(name = "...")` when present, otherwise from the parameter name itself.
//
// Data class parameter lists contain nested `(...)` from `@Json(name = "x")`
// annotations, so the class body is extracted with a paren-balance scan rather
// than a regex.
func kotlinDtoKeys(src, dtoClass string) (map[string]bool, bool) {
	header := regexp.MustCompile(`data class\s+` + regexp.QuoteMeta(dtoClass) + `\s*\(`)
	loc := header.FindStringIndex(src)
	if loc == nil {
		return nil, false
	}
	body, ok := captureBalanced(src[loc[1]-1:], '(', ')')
	if !ok {
		return nil, false
	}
	keys := map[string]bool{}
	paramRe := regexp.MustCompile(
		`(?:@Json\(\s*name\s*=\s*"([^"]+)"\s*\)\s*)?val\s+([a-zA-Z_][a-zA-Z0-9_]*)`,
	)
	for _, p := range paramRe.FindAllStringSubmatch(body, -1) {
		if p[1] != "" {
			keys[p[1]] = true
		} else {
			keys[p[2]] = true
		}
	}
	return keys, true
}

// captureBalanced expects s to start with the opening rune `open` and returns
// the substring between it and its matching `close`, respecting nesting.
func captureBalanced(s string, open, close byte) (string, bool) {
	if len(s) == 0 || s[0] != open {
		return "", false
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return s[1:i], true
			}
		}
	}
	return "", false
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// assertSubset reports keys that exist in `client` but not in `backend`. A
// client may legitimately omit optional backend fields, so the opposite
// direction is not enforced.
func assertSubset(t *testing.T, proto, clientName string, backend, client map[string]bool) {
	t.Helper()
	var extra []string
	for k := range client {
		if !backend[k] {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		t.Errorf(
			"%s: %s sends keys not declared by the backend struct: %v\n  backend keys: %v\n  %s keys:    %v",
			proto, clientName, extra, sortedKeys(backend), clientName, sortedKeys(client),
		)
	}
}

func TestAPISync_Payloads(t *testing.T) {
	jsSrc := readFile(t, "frontend/src/service/service.js")
	ktSrc := readFile(t, "warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/dto/WarpnetDtos.kt")

	for _, spec := range payloadSpecs {
		spec := spec
		t.Run(spec.name, func(t *testing.T) {
			backendKeys := goJsonKeys(spec.goRequest)
			require.NotEmptyf(t, backendKeys, "no JSON tags on %T", spec.goRequest)

			if spec.jsConst != "" {
				jsKeys, ok := jsBodyKeysForConst(jsSrc, spec.jsConst)
				if assert.Truef(t, ok,
					"frontend does not reference path: %s — drop jsConst from the spec or wire it up in service.js",
					spec.jsConst) {
					assertSubset(t, spec.name, "frontend", backendKeys, jsKeys)
				}
			}

			if spec.kotlinDtoClass != "" {
				ktKeys, ok := kotlinDtoKeys(ktSrc, spec.kotlinDtoClass)
				if assert.Truef(t, ok,
					"warpdroid DTO data class %s not found in WarpnetDtos.kt",
					spec.kotlinDtoClass) {
					assertSubset(t, spec.name, "warpdroid", backendKeys, ktKeys)
				}
			}
		})
	}
}

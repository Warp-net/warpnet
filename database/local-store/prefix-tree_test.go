package local_store

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewPrefixBuilder(t *testing.T) {
	ns := NewPrefixBuilder("/TEST")
	assert.Equal(t, Namespace("/TEST"), ns)
}

func TestNewPrefixBuilder_PanicsWithoutSlash(t *testing.T) {
	assert.Panics(t, func() {
		NewPrefixBuilder("TEST")
	})
}

func TestNewPrefixBuilder_PanicsEmpty(t *testing.T) {
	assert.Panics(t, func() {
		NewPrefixBuilder("")
	})
}

func TestNamespace_AddSubPrefix(t *testing.T) {
	ns := NewPrefixBuilder("/TEST").AddSubPrefix("SUB")
	assert.Equal(t, Namespace("/TEST/SUB"), ns)
}

func TestNamespace_AddSubPrefix_PanicsEmpty(t *testing.T) {
	assert.Panics(t, func() {
		NewPrefixBuilder("/TEST").AddSubPrefix("")
	})
}

func TestNamespace_Build(t *testing.T) {
	key := NewPrefixBuilder("/TEST").Build()
	assert.Equal(t, DatabaseKey("/TEST"), key)
}

func TestNamespace_AddRootID(t *testing.T) {
	p := NewPrefixBuilder("/TEST").AddRootID("root1")
	assert.Equal(t, ParentLayer("/TEST/root1"), p)
}

func TestNamespace_AddRootID_PanicsEmpty(t *testing.T) {
	assert.Panics(t, func() {
		NewPrefixBuilder("/TEST").AddRootID("")
	})
}

func TestParentLayer_Build(t *testing.T) {
	key := NewPrefixBuilder("/TEST").AddRootID("root1").Build()
	assert.Equal(t, DatabaseKey("/TEST/root1"), key)
}

func TestParentLayer_AddRange(t *testing.T) {
	rl := NewPrefixBuilder("/TEST").AddRootID("root1").AddRange(FixedRangeKey)
	assert.Contains(t, string(rl), "fixed")
}

func TestParentLayer_AddRange_PanicsEmpty(t *testing.T) {
	assert.Panics(t, func() {
		NewPrefixBuilder("/TEST").AddRootID("root1").AddRange("")
	})
}

func TestParentLayer_AddRange_PanicsInvalidPrefix(t *testing.T) {
	assert.Panics(t, func() {
		NewPrefixBuilder("/TEST").AddRootID("root1").AddRange("notanumber")
	})
}

func TestParentLayer_AddRange_NumericValid(t *testing.T) {
	rl := NewPrefixBuilder("/TEST").AddRootID("root1").AddRange("12345")
	assert.Contains(t, string(rl), "12345")
}

func TestParentLayer_AddParentId(t *testing.T) {
	il := NewPrefixBuilder("/TEST").AddRootID("root1").AddParentId("parent1")
	key := il.Build()
	assert.Equal(t, DatabaseKey("/TEST/root1/parent1"), key)
}

func TestParentLayer_AddParentId_PanicsEmpty(t *testing.T) {
	assert.Panics(t, func() {
		NewPrefixBuilder("/TEST").AddRootID("root1").AddParentId("")
	})
}

func TestParentLayer_AddReversedTimestamp(t *testing.T) {
	tm := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	rl := NewPrefixBuilder("/TEST").AddRootID("root1").AddReversedTimestamp(tm)
	key := rl.Build()
	assert.Contains(t, key.String(), "/TEST/root1/")
}

func TestRangeLayer_Build(t *testing.T) {
	key := NewPrefixBuilder("/TEST").AddRootID("root1").AddRange(FixedRangeKey).Build()
	assert.Equal(t, DatabaseKey("/TEST/root1/fixed"), key)
}

func TestRangeLayer_AddParentId(t *testing.T) {
	key := NewPrefixBuilder("/TEST").AddRootID("root1").AddRange(FixedRangeKey).AddParentId("parent1").Build()
	assert.Equal(t, DatabaseKey("/TEST/root1/fixed/parent1"), key)
}

func TestRangeLayer_AddParentId_PanicsEmpty(t *testing.T) {
	assert.Panics(t, func() {
		NewPrefixBuilder("/TEST").AddRootID("root1").AddRange(FixedRangeKey).AddParentId("")
	})
}

func TestIdLayer_AddId(t *testing.T) {
	key := NewPrefixBuilder("/TEST").AddRootID("root1").AddParentId("parent1").AddId("id1").Build()
	assert.Equal(t, DatabaseKey("/TEST/root1/parent1/id1"), key)
}

func TestIdLayer_AddId_PanicsEmpty(t *testing.T) {
	assert.Panics(t, func() {
		NewPrefixBuilder("/TEST").AddRootID("root1").AddParentId("parent1").AddId("")
	})
}

func TestIdLayer_AddReversedTimestamp(t *testing.T) {
	tm := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	key := NewPrefixBuilder("/TEST").AddRootID("root1").AddParentId("parent1").AddReversedTimestamp(tm).Build()
	assert.Contains(t, key.String(), "/TEST/root1/parent1/")
}

func TestDatabaseKey_IsEmpty(t *testing.T) {
	assert.True(t, DatabaseKey("").IsEmpty())
	assert.False(t, DatabaseKey("some").IsEmpty())
}

func TestDatabaseKey_String(t *testing.T) {
	k := DatabaseKey("/TEST/key")
	assert.Equal(t, "/TEST/key", k.String())
}

func TestDatabaseKey_Bytes(t *testing.T) {
	k := DatabaseKey("/TEST/key")
	assert.Equal(t, []byte("/TEST/key"), k.Bytes())
}

func TestDatabaseKey_DropId(t *testing.T) {
	k := DatabaseKey("/TEST/root/id123")
	assert.Equal(t, "/TEST/root", k.DropId())
}

func TestDatabaseKey_DropId_NoDelimiter(t *testing.T) {
	k := DatabaseKey("noslash")
	assert.Equal(t, "noslash", k.DropId())
}

func TestDatabaseKey_DatastoreKey(t *testing.T) {
	k := DatabaseKey("/TEST/key")
	dsKey := k.DatastoreKey()
	assert.Equal(t, "/TEST/key", dsKey.String())
}

func TestDelimiter(t *testing.T) {
	assert.Equal(t, "/", Delimeter)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "fixed", FixedKey)
	assert.Equal(t, "fixed", string(FixedRangeKey))
	assert.Equal(t, "none", string(NoneRangeKey))
}

func TestReversedTimestamp_IsDecreasing(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	k1 := NewPrefixBuilder("/T").AddRootID("r").AddReversedTimestamp(t1).Build()
	k2 := NewPrefixBuilder("/T").AddRootID("r").AddReversedTimestamp(t2).Build()

	// Earlier time should produce a larger reversed timestamp (sorts later)
	// Later time should produce a smaller reversed timestamp (sorts first)
	assert.True(t, strings.Compare(k2.String(), k1.String()) < 0, "later time should sort first with reversed timestamps")
}

package tool

import "testing"

type testSystemConfigEntry struct {
	key   string
	value string
	typ   string
}

func (e testSystemConfigEntry) ConfigKey() string   { return e.key }
func (e testSystemConfigEntry) ConfigValue() string { return e.value }
func (e testSystemConfigEntry) ConfigType() string  { return e.typ }

func TestSystemConfigSliceReflectToStructUsesNarrowContract(t *testing.T) {
	target := struct {
		Name    string
		Enabled *bool
		Retries int
		Limit   int64
		Labels  []string
	}{}

	SystemConfigSliceReflectToStruct([]testSystemConfigEntry{
		{key: "Name", value: "PPanel", typ: "string"},
		{key: "Enabled", value: "true", typ: "bool"},
		{key: "Retries", value: "3", typ: "int"},
		{key: "Limit", value: "42", typ: "int64"},
		{key: "Labels", value: `["admin","user"]`, typ: "interface"},
	}, &target)

	if target.Name != "PPanel" || target.Enabled == nil || !*target.Enabled || target.Retries != 3 || target.Limit != 42 {
		t.Fatalf("unexpected scalar values: %#v", target)
	}
	if len(target.Labels) != 2 || target.Labels[0] != "admin" || target.Labels[1] != "user" {
		t.Fatalf("unexpected labels: %#v", target.Labels)
	}
}

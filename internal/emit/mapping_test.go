package emit

import (
	"slices"
	"strings"
	"testing"
)

func TestApplyMappings(t *testing.T) {
	m := &Module{Mappings: []Mapping{
		{Module: "Lua", ImportPath: "example.com/lua", Exports: map[string]bool{"StatePop": true, "CState": true}},
		{Module: "CLua", ImportPath: "example.com/lua", Exports: map[string]bool{"CFLua_settop": true}},
		{Module: "Grdb", ImportPath: "example.com/db", Exports: map[string]bool{"Database": true}},
	}}
	src := "func f(L *swiftrt.CState, v lua.Value) {\n\tswiftrt.StatePop(L)\n\tswiftrt.CFLua_settop(L, 0)\n\t_ = swiftrt.Some(clua.Kind(1))\n}\n"
	got, imports := m.applyMappings(src)
	want := "func f(L *lua.CState, v lua.Value) {\n\tlua.StatePop(L)\n\tlua.CFLua_settop(L, 0)\n\t_ = swiftrt.Some(lua.Kind(1))\n}\n"
	if got != want {
		t.Errorf("source:\n%s\nwant:\n%s", got, want)
	}
	if want := []string{`"example.com/lua"`}; !slices.Equal(imports, want) {
		t.Errorf("imports = %v, want %v", imports, want)
	}
}

func TestApplyMappingsLeavesUnmappedNames(t *testing.T) {
	m := &Module{Mappings: []Mapping{{Module: "Lua", ImportPath: "example.com/lua", Exports: map[string]bool{"StatePop": true}}}}
	src := "x := swiftrt.StatePopAll(y)\n"
	if got, imports := m.applyMappings(src); got != src || len(imports) != 0 {
		t.Errorf("got %q, %v", got, imports)
	}
}

func TestPackageExports(t *testing.T) {
	pkg, got, generic, err := PackageExports("testdata/exports")
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "exports" {
		t.Errorf("package %q", pkg)
	}
	if generic["Box"] != 2 || len(generic) != 1 {
		t.Errorf("generic = %v, want Box with 2 type parameters", generic)
	}
	want := map[string]bool{"Exported": true, "Kind": true, "KindA": true, "Limit": true, "Box": true}
	if len(got) != len(want) {
		t.Errorf("exports = %v, want %v", got, want)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing %s in %v", k, got)
		}
	}
}

func TestForeignValueName(t *testing.T) {
	m := &Module{Mappings: []Mapping{{Module: "Lua", Exports: map[string]bool{"CVLUA_REGISTRYINDEX": true, "MultiRet": true, "WrapperUpvalues": true}}}}
	cases := []struct {
		usr, name string
		sig       declSig
		want      string
	}{
		{"c:CLua.h@LUA_REGISTRYINDEX", "LUA_REGISTRYINDEX", declSig{}, "CVLUA_REGISTRYINDEX"},
		{"c:lauxlib.h@2835@macro@LUA_NOREF", "LUA_NOREF", declSig{}, ""},
		{"s:3Lua8MultiRets5Int32Vvp", "MultiRet", declSig{owner: "Lua", base: "MultiRet"}, "MultiRet"},
		{"s:3Lua7WrapperV8upvaluesSivpZ", "upvalues", declSig{owner: "Lua.Wrapper", base: "upvalues"}, "WrapperUpvalues"},
		{"c:objc(cs)NSObject(py)hash", "hash", declSig{}, ""},
	}
	for _, c := range cases {
		if got := m.foreignValueName(c.usr, c.name, c.sig); got != c.want {
			t.Errorf("foreignValueName(%q) = %q, want %q", c.usr, got, c.want)
		}
	}
}

func TestMappedCall(t *testing.T) {
	m := &Module{Mappings: []Mapping{{Module: "Glue", Exports: map[string]bool{"EvensUpTo": true, "Unbox": true}}}}
	for src, want := range map[string]bool{
		"swiftrt.EvensUpTo(7)":        true,
		"swiftrt.Unbox[int](b)":       true,
		"swiftrt.StrideFromToBy(0)":   false,
		"x.EvensUpTo(7)":              false,
		"swiftrt.EvensUpTo(7).Values": false,
	} {
		if got := m.mappedCall(src); got != want {
			t.Errorf("mappedCall(%q) = %v", src, got)
		}
	}
}

func TestOwnGenericParams(t *testing.T) {
	for dem, want := range map[string]string{
		"(extension in Lua):Swift.UnsafeMutablePointer<A where A == __C.lua_State>.checkArgument<A>(Swift.Int32) throws -> A1":                         "A1",
		"(extension in Lua):Swift.UnsafeMutablePointer<A where A == __C.lua_State>.push<A where A1: Lua.Pushable>(_: A1?, toindex: Swift.Int32) -> ()": "A1",
		"static Lua.Metatable.FieldType.property<A>(get: (A) -> A1, set: ((inout A, A1) -> ())?) -> Lua.Metatable<A>.FieldType":                        "A1",
		"static Lua.Metatable.FieldType.memberfn<A, B>((inout A, A1) throws -> B1) -> Lua.Metatable<A>.FieldType":                                      "A1,B1",
		"Glue.unbox<A>(Glue.Box<A>) -> A":                 "A",
		"Glue.pair<A, B>(A, B) -> (A, B)":                 "A,B",
		"Glue.glueAdd(Swift.Int, Swift.Int) -> Swift.Int": "",
	} {
		if got := strings.Join(ownGenericParams(dem), ","); got != want {
			t.Errorf("ownGenericParams(%q) = %q, want %q", dem, got, want)
		}
	}
}

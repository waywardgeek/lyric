package lir

import (
	"fmt"
	"strings"
)

// dedup removes duplicate entries from a slice, keeping the first occurrence.
func dedup[T any](items []T, key func(T) string) []T {
	seen := map[string]bool{}
	var result []T
	for _, item := range items {
		k := key(item)
		if !seen[k] {
			seen[k] = true
			result = append(result, item)
		}
	}
	return result
}

// topoSortStructs sorts struct declarations so that a struct embedding another
// struct by value comes after the embedded struct. This is required because C
// needs complete type definitions for by-value fields.
func topoSortStructs(structs []LStructDecl) []LStructDecl {
	if len(structs) <= 1 {
		return structs
	}
	// Build name→index map
	nameIdx := map[string]int{}
	for i, s := range structs {
		nameIdx[s.Name] = i
	}
	// Build adjacency: if struct A has a field of type B (by value), A depends on B
	deps := make([][]int, len(structs))
	for i, s := range structs {
		for _, f := range s.Fields {
			depName := structTypeName(f.Type)
			if depName == "" {
				continue
			}
			if j, ok := nameIdx[depName]; ok && j != i {
				deps[i] = append(deps[i], j)
			}
		}
	}
	// Kahn's algorithm
	inDeg := make([]int, len(structs))
	for i := range deps {
		for _, j := range deps[i] {
			inDeg[j]++ // j must come before i, but we invert: i depends on j
		}
	}
	// Actually: deps[i] = list of j that i depends on. We need reverse edges.
	// deps[i] has j means i needs j first → j has an edge to i in topo order.
	revDeps := make([][]int, len(structs))
	inDeg2 := make([]int, len(structs))
	for i := range deps {
		for _, j := range deps[i] {
			revDeps[j] = append(revDeps[j], i)
			inDeg2[i]++
		}
	}
	var queue []int
	for i, d := range inDeg2 {
		if d == 0 {
			queue = append(queue, i)
		}
	}
	var result []LStructDecl
	for len(queue) > 0 {
		idx := queue[0]
		queue = queue[1:]
		result = append(result, structs[idx])
		for _, next := range revDeps[idx] {
			inDeg2[next]--
			if inDeg2[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	// If cycle or missed items, append remaining
	if len(result) < len(structs) {
		added := map[string]bool{}
		for _, s := range result {
			added[s.Name] = true
		}
		for _, s := range structs {
			if !added[s.Name] {
				result = append(result, s)
			}
		}
	}
	return result
}

// structTypeName returns the struct name if the type is a named struct type, else "".
func structTypeName(t *LType) string {
	if t == nil {
		return ""
	}
	if t.Kind == LTyStruct {
		return t.Name
	}
	return ""
}

// EmitC generates C source code from a monomorphized LIR program.
// The program MUST be monomorphized before calling this — C has no generics.
func EmitC(prog *LProgram) string {
	g := &cGen{
		prog:        prog,
		indent:      0,
		tempUsed:    map[int]bool{},
		tempTypes:   map[int]*LType{},
		varTypes:    map[string]*LType{},
		sliceTypes:  map[string]string{},
		optTypes:    map[string]string{},
		resultTypes: map[string]string{},
		chanTypes:   map[string]string{},
		lambdaID:    0,
		lambdas:     nil,
		ifaceByName: map[string]*LInterfaceDecl{},
		implMap:     map[string][]string{}, // class → []interface
		funcByName:  map[string]*LFuncDecl{},
		simpleEnums: map[string]bool{},
		tupleTypes:  map[string]string{},
	}
	// Build interface lookup
	for i := range prog.Interfaces {
		g.ifaceByName[prog.Interfaces[i].Name] = &prog.Interfaces[i]
	}
	// Build implements map from class declarations
	for _, c := range prog.Classes {
		if len(c.Implements) > 0 {
			g.implMap[c.Name] = c.Implements
		}
	}
	// Build function lookup
	for i := range prog.Functions {
		f := &prog.Functions[i]
		key := f.Name
		if f.Receiver != "" {
			key = f.Receiver + "." + f.Name
		}
		g.funcByName[key] = f
	}
	return g.generate()
}

type cGen struct {
	prog        *LProgram
	buf         strings.Builder
	indent      int
	tempUsed    map[int]bool
	tempTypes   map[int]*LType    // resolved types for temps
	varTypes    map[string]*LType // resolved types for variables (overrides type-var types)
	sliceTypes  map[string]string // cType(elem) → typedef name
	optTypes    map[string]string // cType(elem) → typedef name
	resultTypes map[string]string // cType(elem) → typedef name
	sliceCount  int
	optCount    int
	resultCount int
	lambdaID    int
	lambdas     []cLambda
	currentFunc *LFuncDecl // tracks current function for return type info

	// Generator state machine support
	inGenerator   bool   // true when emitting a generator's next function
	genStructName string // name of the current generator's state struct type
	genYieldCount int    // counter for yield state labels

	// Channel support
	chanTypes map[string]string // cType(elem) → suffix name (e.g. "i32")
	chanCount int

	// Spawn support
	spawnID       int
	spawnFuncs    []cSpawnFunc
	spawnCaptures map[string]bool // variables captured by current spawn block (accessed via _ctx->)

	// Interface vtable dispatch
	ifaceByName map[string]*LInterfaceDecl // interface name → decl
	implMap     map[string][]string        // class name → interfaces it implements
	funcByName  map[string]*LFuncDecl      // "name" or "Class.method" → func decl

	// OS builtin helpers needed
	needsOsArgs   bool
	needsExecCmd  bool
	needsPathJoin bool

	// Simple enum tracking (all-unit-variant enums emitted as C enum, not tagged union)
	simpleEnums map[string]bool

	// Tuple type typedefs (like sliceTypes/optTypes pattern)
	tupleTypes map[string]string // "fields_sig" → typedef name
	tupleCount int

	// mut param tracking (per-function)
	mutParams map[string]bool // parameter names that are mut (passed as pointers)
}

type cSpawnFunc struct {
	name     string
	bodyStr  string
	captures []cCapture
}

type cCapture struct {
	name string
	typ  string
}

type cLambda struct {
	name    string
	retType string
	params  string
	bodyStr string
}

// ---------------------------------------------------------------------------
// Top-level generation
// ---------------------------------------------------------------------------

func (g *cGen) generate() string {
	g.line("/* Generated by Forge compiler — C backend */")
	g.line("#include <stdio.h>")
	g.line("#include <stdlib.h>")
	g.line("#include <stdint.h>")
	g.line("#include <stdbool.h>")
	g.line("#include <string.h>")
	g.line("#include <stdarg.h>")
	g.line("#include <setjmp.h>")
	g.line(`#include "forge_runtime.h"`)
	g.line("")
	// Test assertion support (always emitted — small cost, simpler than conditional)
	g.line("static jmp_buf _forge_test_jmp;")
	g.line("static int _forge_test_failed;")
	g.line(`#define forge_assert(cond, msg, file, line) do { \`)
	g.line(`    if (!(cond)) { \`)
	g.line(`        fprintf(stderr, "  assert failed at %s:%d\n    %.*s\n", file, line, (int)(msg).len, (const char*)(msg).data); \`)
	g.line(`        _forge_test_failed = 1; \`)
	g.line(`        longjmp(_forge_test_jmp, 1); \`)
	g.line(`    } \`)
	g.line(`} while(0)`)
	g.line(`#define forge_assert_eq(eq, actual_str, expected_str, msg, file, line) do { \`)
	g.line(`    if (!(eq)) { \`)
	g.line(`        forge_string _a = (actual_str); \`)
	g.line(`        forge_string _e = (expected_str); \`)
	g.line(`        fprintf(stderr, "  assert_eq failed at %s:%d\n    %.*s\n    expected: %.*s\n    got:      %.*s\n", \`)
	g.line(`            file, line, (int)(msg).len, (const char*)(msg).data, \`)
	g.line(`            (int)_e.len, (const char*)_e.data, (int)_a.len, (const char*)_a.data); \`)
	g.line(`        _forge_test_failed = 1; \`)
	g.line(`        longjmp(_forge_test_jmp, 1); \`)
	g.line(`    } \`)
	g.line(`} while(0)`)
	g.line("")

	// Pre-scan to collect all composite types used in the program
	g.collectCompositeTypes()

	// Deduplicate types (multi-file compilation can produce duplicates)
	g.prog.Structs = dedup(g.prog.Structs, func(s LStructDecl) string { return s.Name })
	g.prog.Classes = dedup(g.prog.Classes, func(c LClassDecl) string { return c.Name })
	g.prog.Enums = dedup(g.prog.Enums, func(e LEnumDecl) string { return e.Name })
	g.prog.Functions = dedup(g.prog.Functions, func(f LFuncDecl) string {
		if f.Receiver != "" {
			return f.Receiver + "." + f.Name
		}
		return f.Name
	})

	// Forward-declare structs, classes, and enums BEFORE composite types
	for _, s := range g.prog.Structs {
		g.linef("typedef struct %s %s;", g.structName(s.Name, s.IsExported), g.structName(s.Name, s.IsExported))
	}
	for _, c := range g.prog.Classes {
		g.linef("typedef struct %s %s;", g.structName(c.Name, c.IsExported), g.structName(c.Name, c.IsExported))
	}
	for _, e := range g.prog.Enums {
		if isSimpleEnum(&e) {
			// Simple enums: emit complete typedef enum definition here
			// so they're available as complete types for FORGE_OPT_DEF below
			g.emitSimpleEnumDecl(&e)
		} else {
			g.linef("typedef struct %s %s;", g.structName(e.Name, e.IsExported), g.structName(e.Name, e.IsExported))
		}
	}
	if len(g.prog.Structs)+len(g.prog.Classes)+len(g.prog.Enums) > 0 {
		g.line("")
	}

	// Emit slice type typedefs (only need forward declarations — uses ElemType*)
	// Skip ForgeSlice_uint8_t — already defined in runtime as forge_string's underlying type
	for elemType, name := range g.sliceTypes {
		if name == "ForgeSlice_uint8_t" || name == "ForgeSlice_forge_string" {
			continue
		}
		g.linef("FORGE_SLICE_DEF(%s, %s)", elemType, name)
	}
	if len(g.sliceTypes) > 0 {
		g.line("")
	}

	// Pre-collect optional types from struct and class fields so FORGE_OPT_DEF
	// can be emitted before structs that reference ForgeOpt_* types as fields.
	// Without this, struct LExpr references ForgeOpt_LBinOpData before it's defined.
	for _, s := range g.prog.Structs {
		for _, f := range s.Fields {
			if f.Type != nil && f.Type.Kind == LTyOptional {
				g.optTypeName(f.Type.Elem)
			}
		}
	}
	for _, c := range g.prog.Classes {
		for _, f := range c.Fields {
			if f.Type != nil && f.Type.Kind == LTyOptional {
				g.optTypeName(f.Type.Elem)
			}
		}
	}

	// Emit enum definitions first (other types may reference them)
	// Simple enums already emitted in forward-decl section as typedef enum
	for _, e := range g.prog.Enums {
		if !isSimpleEnum(&e) {
			g.emitEnumDecl(&e)
		}
	}

	// Emit struct definitions (topologically sorted so embedded structs come first)
	g.prog.Structs = topoSortStructs(g.prog.Structs)
	for _, s := range g.prog.Structs {
		g.emitStructDecl(&s)
	}

	// Emit optional/result type typedefs AFTER struct/enum definitions but BEFORE
	// class definitions. These macros embed the element type by value, so the inner
	// type must be complete. Structs like LExpr have ForgeOpt_* fields, so opt defs
	// must come before any struct/class that references them.
	// We pre-collected struct/class field optionals above; emitStructDecl/emitClassDecl
	// may register more during field emission.
	emittedOpts := make(map[string]bool)
	for elemType, name := range g.optTypes {
		g.linef("FORGE_OPT_DEF(%s, %s)", elemType, name)
		emittedOpts[elemType] = true
	}
	emittedResults := make(map[string]bool)
	for elemType, name := range g.resultTypes {
		g.linef("FORGE_RESULT_DEF(%s, %s)", elemType, name)
		emittedResults[elemType] = true
	}
	if len(g.optTypes)+len(g.resultTypes) > 0 {
		g.line("")
	}

	// Emit class definitions (heap-allocated structs)
	for _, c := range g.prog.Classes {
		g.emitClassDecl(&c)
	}

	// Emit any additional optional/result/channel types discovered during class/func emission
	for elemType, name := range g.optTypes {
		if !emittedOpts[elemType] {
			g.linef("FORGE_OPT_DEF(%s, %s)", elemType, name)
		}
	}
	for elemType, name := range g.resultTypes {
		if !emittedResults[elemType] {
			g.linef("FORGE_RESULT_DEF(%s, %s)", elemType, name)
		}
	}
	for cElemType, suffix := range g.chanTypes {
		chanName := fmt.Sprintf("ForgeChan_%s", suffix)
		g.linef("FORGE_CHAN_DEF(%s, %s)", cElemType, chanName)
	}
	if len(g.optTypes)+len(g.resultTypes)+len(g.chanTypes) > 0 {
		g.line("")
	}

	// Emit generator state struct typedefs
	for i := range g.prog.Functions {
		f := &g.prog.Functions[i]
		if g.isGeneratorFunc(f) {
			g.emitGeneratorStructDecl(f)
		}
	}

	// Emit interface vtable and boxed types (skip generic interfaces)
	for _, iface := range g.prog.Interfaces {
		if len(iface.TypeParams) > 0 {
			continue // Generic interfaces are handled through concrete methods directly
		}
		ifName := g.structName(iface.Name, iface.IsExported)
		// Vtable struct: function pointers for each method
		g.linef("typedef struct %s_vtable {", ifName)
		g.indent++
		for _, m := range iface.Methods {
			retType := g.cType(m.ReturnType)
			var paramTypes []string
			paramTypes = append(paramTypes, "void*") // self
			for _, p := range m.Params {
				paramTypes = append(paramTypes, g.cType(p.Type))
			}
			g.linef("%s (*%s)(%s);", retType, m.Name, strings.Join(paramTypes, ", "))
		}
		g.indent--
		g.linef("} %s_vtable;", ifName)
		// Boxed interface struct: data pointer + vtable pointer
		g.linef("typedef struct %s {", ifName)
		g.indent++
		g.line("void* _data;")
		g.linef("const %s_vtable* _vtable;", ifName)
		g.indent--
		g.linef("} %s;", ifName)
		g.line("")
	}

	// Emit static vtable instances for each (class, interface) pair
	for _, c := range g.prog.Classes {
		className := g.structName(c.Name, c.IsExported)
		for _, ifaceName := range c.Implements {
			iface, ok := g.ifaceByName[ifaceName]
			if !ok {
				continue
			}
			ifName := g.structName(iface.Name, iface.IsExported)
			g.linef("static const %s_vtable %s_as_%s;", ifName, className, ifName)
		}
	}

	// Emit type aliases
	for _, td := range g.prog.TypeDefs {
		g.linef("typedef %s %s;", g.cType(td.Type), g.visName(td.Name, td.IsExported))
		g.line("")
	}

	// Auto-generate to_string functions for enums, structs, and classes (used by assert_eq)
	g.emitToStringFunctions()

	// Pre-scan function signatures to discover tuple types (needed before forward decls)
	for _, f := range g.prog.Functions {
		if len(f.TypeParams) > 0 {
			continue
		}
		if f.ReturnType != nil && f.ReturnType.Kind == LTyTuple {
			g.cTupleType(f.ReturnType)
		}
		for _, p := range f.Params {
			if p.Type != nil && p.Type.Kind == LTyTuple {
				g.cTupleType(p.Type)
			}
		}
	}

	// Emit tuple typedefs (before forward declarations that reference them)
	for key, name := range g.tupleTypes {
		fieldTypes := strings.Split(key, ",")
		g.linef("typedef struct %s {", name)
		g.indent++
		emitted := false
		for i, ft := range fieldTypes {
			if ft != "" {
				g.linef("%s _%d;", ft, i)
				emitted = true
			}
		}
		if !emitted {
			g.line("char _dummy;")
		}
		g.indent--
		g.linef("} %s;", name)
	}
	if len(g.tupleTypes) > 0 {
		g.line("")
	}

	// Forward-declare all functions (including methods)
	// Skip unmonomorphized generic functions (still have TypeParams — dead code from stdlib merge)
	for _, f := range g.prog.Functions {
		if g.funcName(&f) == "main" {
			continue
		}
		if len(f.TypeParams) > 0 {
			continue
		}
		g.emitFuncForwardDecl(&f)
	}
	if len(g.prog.Functions) > 0 {
		g.line("")
	}

	// Emit global constants/variables
	for _, gv := range g.prog.Globals {
		cType := g.cType(gv.Type)
		if gv.Init != nil {
			g.linef("static %s %s = %s;", cType, cSafeName(gv.Name), g.emitValue(gv.Init))
		} else {
			g.linef("static %s %s;", cType, cSafeName(gv.Name))
		}
	}
	if len(g.prog.Globals) > 0 {
		g.line("")
	}

	// Emit vtable definitions (after forward decls so method names are known)
	for _, c := range g.prog.Classes {
		className := g.structName(c.Name, c.IsExported)
		for _, ifaceName := range c.Implements {
			iface, ok := g.ifaceByName[ifaceName]
			if !ok {
				continue
			}
			ifName := g.structName(iface.Name, iface.IsExported)
			g.linef("static const %s_vtable %s_as_%s = {", ifName, className, ifName)
			g.indent++
			for _, m := range iface.Methods {
				// Build the cast type for the function pointer
				retType := g.cType(m.ReturnType)
				var paramTypes []string
				paramTypes = append(paramTypes, "void*")
				for _, p := range m.Params {
					paramTypes = append(paramTypes, g.cType(p.Type))
				}
				castType := fmt.Sprintf("%s(*)(%s)", retType, strings.Join(paramTypes, ", "))
				g.linef(".%s = (%s)%s_%s,", m.Name, castType, className, m.Name)
			}
			g.indent--
			g.linef("};")
		}
	}
	if len(g.prog.Classes) > 0 {
		g.line("")
	}

	// Emit function bodies (lambdas are collected during this pass)
	funcBuf := g.buf
	g.buf = strings.Builder{}
	for _, f := range g.prog.Functions {
		if len(f.TypeParams) > 0 {
			continue // skip unmonomorphized generic functions
		}
		g.emitFuncDecl(&f)
	}
	funcCode := g.buf.String()
	g.buf = funcBuf

	// Emit hoisted lambdas before function bodies
	for _, lam := range g.lambdas {
		g.linef("static %s %s(%s) {", lam.retType, lam.name, lam.params)
		g.buf.WriteString(lam.bodyStr)
		g.line("}")
		g.line("")
	}

	// Emit channel implementations (function definitions — must come after type decls)
	for cElemType, suffix := range g.chanTypes {
		chanName := fmt.Sprintf("ForgeChan_%s", suffix)
		g.linef("FORGE_CHAN_IMPL(%s, %s, %s)", cElemType, chanName, suffix)
		g.line("")
	}

	// Emit spawn wrapper functions
	for _, sf := range g.spawnFuncs {
		if len(sf.captures) > 0 {
			g.linef("typedef struct %s_ctx {", sf.name)
			g.indent++
			for _, c := range sf.captures {
				g.linef("%s* %s;", c.typ, c.name) // pointer to original variable
			}
			g.indent--
			g.linef("} %s_ctx;", sf.name)
		}
		g.linef("static void* %s(void* _arg) {", sf.name)
		g.indent++
		if len(sf.captures) > 0 {
			g.linef("%s_ctx* _ctx = (%s_ctx*)_arg;", sf.name, sf.name)
			// No local copies — body accesses via (*_ctx->name)
		}
		g.buf.WriteString(sf.bodyStr)
		if len(sf.captures) > 0 {
			g.linef("free(_ctx);")
		}
		g.line("return NULL;")
		g.indent--
		g.line("}")
		g.line("")
	}

	// Emit OS builtin helpers (after slice type defs are available)
	g.emitOsHelpers()

	// Now emit function bodies
	g.buf.WriteString(funcCode)

	return g.buf.String()
}

// cTupleType returns a C type for a tuple. For known patterns like (string, bool)
// that have typedef'd types in the runtime, returns the typedef name instead of
// an anonymous struct (which would be type-incompatible with runtime functions).
func (g *cGen) cTupleType(t *LType) string {
	if t.Kind == LTyTuple && len(t.Fields) == 2 {
		if t.Fields[0].Type != nil && t.Fields[0].Type.Kind == LTyString &&
			t.Fields[1].Type != nil && t.Fields[1].Type.Kind == LTyBool {
			return "forge_str_bool_t"
		}
		if t.Fields[0].Type != nil && t.Fields[0].Type.Kind == LTyI64 &&
			t.Fields[1].Type != nil && t.Fields[1].Type.Kind == LTyBool {
			return "forge_atoi_result"
		}
		if t.Fields[0].Type != nil && t.Fields[0].Type.Kind == LTyF64 &&
			t.Fields[1].Type != nil && t.Fields[1].Type.Kind == LTyBool {
			return "forge_parse_float_result"
		}
	}
	// Fallback: register a named typedef (like sliceTypes/optTypes pattern)
	var fieldTypes []string
	for _, f := range t.Fields {
		fieldTypes = append(fieldTypes, g.cType(f.Type))
	}
	key := strings.Join(fieldTypes, ",")
	if name, ok := g.tupleTypes[key]; ok {
		return name
	}
	name := fmt.Sprintf("ForgeTuple_%d", g.tupleCount)
	g.tupleCount++
	g.tupleTypes[key] = name
	return name
}

// ---------------------------------------------------------------------------
// OS builtin helpers (emitted inline, after slice type defs are available)

func (g *cGen) emitOsHelpers() {
	sliceType := g.sliceTypeName(&LType{Kind: LTyString})

	if g.needsOsArgs {
		g.linef("static inline %s _forge_os_args(int argc, char** argv) {", sliceType)
		g.indent++
		g.linef("%s result = {0};", sliceType)
		g.linef("result.data = (forge_string*)malloc(sizeof(forge_string) * argc);")
		g.line("result.len = (int32_t)argc;")
		g.line("result.cap = (int32_t)argc;")
		g.line("for (int i = 0; i < argc; i++) result.data[i] = forge_str_from_cstr(argv[i]);")
		g.line("return result;")
		g.indent--
		g.line("}")
		g.line("")
	}

	if g.needsExecCmd {
		g.linef("static inline forge_str_bool_t _forge_exec_command(forge_string program, %s args) {", sliceType)
		g.indent++
		g.line("size_t total = program.len + 1;")
		g.line("for (int i = 0; i < args.len; i++) total += args.data[i].len + 3;")
		g.line("char* cmd = (char*)malloc(total + 16);")
		g.line("memcpy(cmd, program.data, program.len); cmd[program.len] = '\\0';")
		g.line(`for (int i = 0; i < args.len; i++) { strcat(cmd, " "); strncat(cmd, (const char*)args.data[i].data, args.data[i].len); }`)
		g.line(`strcat(cmd, " 2>&1");`)
		g.line(`FILE* fp = popen(cmd, "r");`)
		g.line("free(cmd);")
		g.line(`if (!fp) { forge_str_bool_t r = {FORGE_STR_EMPTY, false}; return r; }`)
		g.line("size_t cap = 4096, len = 0;")
		g.line("uint8_t* out = (uint8_t*)malloc(cap);")
		g.line("size_t n;")
		g.line("while ((n = fread(out + len, 1, cap - len - 1, fp)) > 0) {")
		g.indent++
		g.line("len += n;")
		g.line("if (len + 1 >= cap) { cap *= 2; out = (uint8_t*)realloc(out, cap); }")
		g.indent--
		g.line("}")
		g.line("out[len] = '\\0';")
		g.line("int status = pclose(fp);")
		g.line("forge_str_bool_t r = {{.data = out, .len = (int32_t)len, .cap = (int32_t)len}, status == 0};")
		g.line("return r;")
		g.indent--
		g.line("}")
		g.line("")
	}

	if g.needsPathJoin {
		g.linef("static inline forge_string _forge_path_join(%s parts) {", sliceType)
		g.indent++
		g.line(`if (parts.len == 0) return FORGE_STR_EMPTY;`)
		g.line("int32_t total = 0;")
		g.line("for (int i = 0; i < parts.len; i++) total += parts.data[i].len + 1;")
		g.line("uint8_t* out = (uint8_t*)malloc(total + 1);")
		g.line("int32_t pos = 0;")
		g.line("for (int i = 0; i < parts.len; i++) {")
		g.indent++
		g.line(`if (i > 0) { out[pos++] = '/'; }`)
		g.line("memcpy(out + pos, parts.data[i].data, parts.data[i].len);")
		g.line("pos += parts.data[i].len;")
		g.indent--
		g.line("}")
		g.line("out[pos] = '\\0';")
		g.line("return (forge_string){.data = out, .len = pos, .cap = pos};")
		g.indent--
		g.line("}")
		g.line("")
	}
}

// isSimpleEnum returns true if all variants are unit variants (no associated data).
// Simple enums can be emitted as plain C enums instead of tagged union structs.
func isSimpleEnum(e *LEnumDecl) bool {
	for _, v := range e.Variants {
		if len(v.Fields) > 0 {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Type declarations
// ---------------------------------------------------------------------------

func (g *cGen) emitStructDecl(s *LStructDecl) {
	name := g.structName(s.Name, s.IsExported)
	g.linef("struct %s {", name)
	g.indent++
	if len(s.Fields) == 0 {
		g.line("int _empty; /* C requires at least one field */")
	}
	for _, f := range s.Fields {
		g.linef("%s;", g.cFieldDecl(f.Type, f.Name))
	}
	g.indent--
	g.line("};")
	g.line("")
}

func (g *cGen) emitClassDecl(c *LClassDecl) {
	name := g.structName(c.Name, c.IsExported)
	g.linef("struct %s {", name)
	g.indent++
	if len(c.Fields) == 0 {
		g.line("int _empty;")
	}
	for _, f := range c.Fields {
		g.linef("%s;", g.cFieldDecl(f.Type, f.Name))
	}
	g.indent--
	g.line("};")
	g.line("")
}

func (g *cGen) emitSimpleEnumDecl(e *LEnumDecl) {
	name := g.structName(e.Name, e.IsExported)
	g.simpleEnums[name] = true
	g.linef("typedef enum {")
	g.indent++
	for i, v := range e.Variants {
		comma := ","
		if i == len(e.Variants)-1 {
			comma = ""
		}
		g.linef("%s_%s = %d%s", name, v.Name, v.Tag, comma)
	}
	g.indent--
	g.linef("} %s;", name)
	g.line("")
}

func (g *cGen) emitEnumDecl(e *LEnumDecl) {
	name := g.structName(e.Name, e.IsExported)
	// Emit tag enum
	g.linef("enum %s_Tag {", name)
	g.indent++
	for i, v := range e.Variants {
		comma := ","
		if i == len(e.Variants)-1 {
			comma = ""
		}
		g.linef("%s_%s = %d%s", name, v.Name, v.Tag, comma)
	}
	g.indent--
	g.line("};")
	g.line("")

	// Emit variant data structs
	for _, v := range e.Variants {
		if len(v.Fields) > 0 {
			g.linef("typedef struct {")
			g.indent++
			for _, f := range v.Fields {
				g.linef("%s;", g.cFieldDecl(f.Type, f.Name))
			}
			g.indent--
			g.linef("} %s_%s_Data;", name, v.Name)
			g.line("")
		}
	}

	// Emit the tagged union
	g.linef("struct %s {", name)
	g.indent++
	g.linef("enum %s_Tag tag;", name)
	g.line("union {")
	g.indent++
	for _, v := range e.Variants {
		if len(v.Fields) > 0 {
			g.linef("%s_%s_Data %s;", name, v.Name, cSafeName(strings.ToLower(v.Name)))
		}
	}
	g.indent--
	g.line("} data;")
	g.indent--
	g.line("};")
	g.line("")
}

// ---------------------------------------------------------------------------
// Function declarations
// ---------------------------------------------------------------------------

func (g *cGen) emitFuncForwardDecl(f *LFuncDecl) {
	name := g.funcName(f)
	if name == "main" {
		return
	}
	if g.isGeneratorFunc(f) {
		g.emitGeneratorForwardDecls(f)
		return
	}
	retType := g.cReturnType(f.ReturnType)
	params := g.cParamList(f)
	g.linef("%s %s(%s);", retType, name, params)
}

func (g *cGen) emitFuncDecl(f *LFuncDecl) {
	g.currentFunc = f
	if g.isGeneratorFunc(f) {
		g.emitGeneratorFuncDecl(f)
		return
	}
	// Clear per-function type maps (not for generators — they share state across init/next)
	g.tempTypes = map[int]*LType{}
	g.varTypes = map[string]*LType{}
	retType := g.cReturnType(f.ReturnType)
	params := g.cParamList(f)
	name := g.funcName(f)

	if name == "main" {
		g.linef("int main(int _argc, char** _argv) {")
	} else {
		g.linef("%s %s(%s) {", retType, name, params)
	}
	g.indent++
	// Register param types and mut params for format specifier resolution
	g.mutParams = map[string]bool{}
	for _, p := range f.Params {
		if p.Type != nil {
			g.varTypes[p.Name] = p.Type
		}
		if p.Mutable {
			g.mutParams[p.Name] = true
		}
	}
	g.emitStmts(f.Body)
	if name == "main" {
		g.line("return 0;")
	}
	g.indent--
	g.line("}")
	g.line("")
}

func (g *cGen) funcName(f *LFuncDecl) string {
	if f.Receiver != "" {
		return f.Receiver + "_" + f.Name
	}
	if f.Name == "Main" {
		return "main"
	}
	return cSafeName(f.Name)
}

func (g *cGen) cParamList(f *LFuncDecl) string {
	var parts []string
	// For methods, ensure self is the first parameter.
	// Direct methods already have self in Params; impl wrappers don't.
	hasSelf := len(f.Params) > 0 && f.Params[0].Name == "self"
	if f.Receiver != "" && !hasSelf {
		selfType := &LType{Kind: LTyClassHandle, Name: f.Receiver}
		parts = append(parts, g.cFieldDecl(selfType, "self"))
	}
	for _, p := range f.Params {
		decl := g.cFieldDecl(p.Type, p.Name)
		if p.Mutable {
			// mut params are passed by pointer
			decl = g.cType(p.Type) + "* " + p.Name
		}
		parts = append(parts, decl)
	}
	if len(parts) == 0 {
		return "void"
	}
	return strings.Join(parts, ", ")
}

func (g *cGen) cReturnType(t *LType) string {
	if t == nil {
		return "void"
	}
	return g.cType(t)
}

// ---------------------------------------------------------------------------
// Generator state machine support
// ---------------------------------------------------------------------------

// isGeneratorFunc returns true if the function is a generator (returns gen T).
func (g *cGen) isGeneratorFunc(f *LFuncDecl) bool {
	return f.ReturnType != nil && f.ReturnType.Kind == LTyGenerator
}

// genFuncBaseName returns the base name for generator struct/functions.
func (g *cGen) genFuncBaseName(f *LFuncDecl) string {
	name := g.funcName(f)
	return name
}

// collectGenVars recursively scans statements for LStmtVarDecl and returns them.
func (g *cGen) collectGenVars(stmts []LStmt) []LVarDecl {
	var vars []LVarDecl
	for i := range stmts {
		s := &stmts[i]
		switch s.Kind {
		case LStmtVarDecl:
			d := s.Data.(*LVarDecl)
			vars = append(vars, *d)
		case LStmtWhile:
			d := s.Data.(*LWhile)
			vars = append(vars, g.collectGenVars(d.Body)...)
		case LStmtIf:
			d := s.Data.(*LIf)
			vars = append(vars, g.collectGenVars(d.Then)...)
			vars = append(vars, g.collectGenVars(d.Else)...)
		case LStmtFor:
			d := s.Data.(*LFor)
			vars = append(vars, g.collectGenVars(d.Body)...)
		case LStmtBlock:
			d := s.Data.(*LBlock)
			vars = append(vars, g.collectGenVars(d.Stmts)...)
		}
	}
	return vars
}

// countYields counts the number of LStmtYield in a statement tree.
func (g *cGen) countYields(stmts []LStmt) int {
	count := 0
	for i := range stmts {
		s := &stmts[i]
		switch s.Kind {
		case LStmtYield:
			count++
		case LStmtWhile:
			d := s.Data.(*LWhile)
			count += g.countYields(d.Body)
		case LStmtIf:
			d := s.Data.(*LIf)
			count += g.countYields(d.Then)
			count += g.countYields(d.Else)
		case LStmtFor:
			d := s.Data.(*LFor)
			count += g.countYields(d.Body)
		case LStmtBlock:
			d := s.Data.(*LBlock)
			count += g.countYields(d.Stmts)
		}
	}
	return count
}

// emitGeneratorStructDecl emits the typedef for a generator's state struct.
func (g *cGen) emitGeneratorStructDecl(f *LFuncDecl) {
	baseName := g.genFuncBaseName(f)
	structName := baseName + "_gen_t"
	elemType := g.cType(f.ReturnType.Elem)

	g.linef("typedef struct %s {", structName)
	g.indent++
	g.linef("int _state;")
	g.linef("%s _value;", elemType)
	// Params
	for _, p := range f.Params {
		g.linef("%s %s;", g.cType(p.Type), cSafeName(p.Name))
	}
	// Local variables
	vars := g.collectGenVars(f.Body)
	seen := map[string]bool{}
	for _, v := range vars {
		if !seen[v.Name] {
			seen[v.Name] = true
			g.linef("%s %s;", g.cType(v.Type), cSafeName(v.Name))
		}
	}
	g.indent--
	g.linef("} %s;", structName)
	g.line("")
}

// emitGeneratorForwardDecls emits forward declarations for generator init and next functions.
func (g *cGen) emitGeneratorForwardDecls(f *LFuncDecl) {
	baseName := g.genFuncBaseName(f)
	structName := baseName + "_gen_t"

	// Init function: takes same params as original, returns struct pointer
	params := g.cParamList(f)
	g.linef("%s* %s_init(%s);", structName, baseName, params)
	g.linef("bool %s_next(%s* _gen);", baseName, structName)
}

// emitGeneratorFuncDecl emits init + next functions for a generator.
func (g *cGen) emitGeneratorFuncDecl(f *LFuncDecl) {
	baseName := g.genFuncBaseName(f)
	structName := baseName + "_gen_t"
	params := g.cParamList(f)

	// --- Init function ---
	g.linef("%s* %s_init(%s) {", structName, baseName, params)
	g.indent++
	g.linef("%s* _gen = (%s*)malloc(sizeof(%s));", structName, structName, structName)
	g.linef("_gen->_state = 0;")
	// Copy params into struct
	for _, p := range f.Params {
		name := cSafeName(p.Name)
		g.linef("_gen->%s = %s;", name, name)
	}
	// Initialize local variables — only those with literal init values.
	// Vars initialized from temps (computed expressions) will be assigned
	// during body execution in the next() function.
	vars := g.collectGenVars(f.Body)
	seen := map[string]bool{}
	for _, v := range vars {
		if !seen[v.Name] {
			seen[v.Name] = true
			if v.Init != nil && v.Init.Kind != LValTemp {
				g.linef("_gen->%s = %s;", cSafeName(v.Name), g.emitValue(v.Init))
			} else if v.Init == nil {
				g.linef("_gen->%s = 0;", cSafeName(v.Name))
			}
			// Skip vars with temp inits — they'll be set in next()
		}
	}
	g.linef("return _gen;")
	g.indent--
	g.line("}")
	g.line("")

	// --- Next function ---
	g.linef("bool %s_next(%s* _gen) {", baseName, structName)
	g.indent++

	// State dispatch: switch on _gen->_state with gotos into the body
	nYields := g.countYields(f.Body)
	g.linef("switch (_gen->_state) {")
	g.linef("case 0: goto _gen_s0;")
	for i := 1; i <= nYields; i++ {
		g.linef("case %d: goto _gen_s%d;", i, i)
	}
	g.linef("default: return false;")
	g.linef("}")

	// Emit body with generator mode active
	g.inGenerator = true
	g.genStructName = structName
	g.genYieldCount = 0
	g.linef("_gen_s0:;") // entry point label

	// Emit body — skip VarDecls (already initialized in init)
	g.emitGenStmts(f.Body)

	g.linef("return false;") // generator exhausted
	g.inGenerator = false
	g.indent--
	g.line("}")
	g.line("")
}

// emitGenStmts emits statements for generator body, rewriting vars to _gen-> access.
func (g *cGen) emitGenStmts(stmts []LStmt) {
	for i := range stmts {
		g.emitStmt(&stmts[i])
	}
}

// resolveGenBaseName traces a collection LValue back to the generator function name.
// The collection is typically a temp whose expression is a function call returning gen T.
func (g *cGen) resolveGenBaseName(v *LValue) string {
	if v.Kind == LValTemp {
		// Look through the current function's body for the temp definition
		return g.findGenFuncNameInStmts(g.currentFunc.Body, v.TempID)
	}
	return "unknown_gen"
}

// findGenFuncNameInStmts searches statements (recursively into nested blocks)
// for a TempDef with the given ID that is a function call, returning the function name.
func (g *cGen) findGenFuncNameInStmts(stmts []LStmt, tempID int) string {
	for i := range stmts {
		s := &stmts[i]
		switch s.Kind {
		case LStmtTempDef:
			d := s.Data.(*LTempDef)
			if d.ID == tempID {
				return g.extractFuncNameFromExpr(&d.Expr)
			}
		case LStmtIf:
			d := s.Data.(*LIf)
			if r := g.findGenFuncNameInStmts(d.Then, tempID); r != "unknown_gen" {
				return r
			}
			if r := g.findGenFuncNameInStmts(d.Else, tempID); r != "unknown_gen" {
				return r
			}
		case LStmtWhile:
			d := s.Data.(*LWhile)
			if r := g.findGenFuncNameInStmts(d.CondBlock, tempID); r != "unknown_gen" {
				return r
			}
			if r := g.findGenFuncNameInStmts(d.Body, tempID); r != "unknown_gen" {
				return r
			}
		case LStmtFor:
			d := s.Data.(*LFor)
			if r := g.findGenFuncNameInStmts(d.Body, tempID); r != "unknown_gen" {
				return r
			}
		case LStmtSwitch:
			d := s.Data.(*LSwitch)
			for j := range d.Cases {
				if r := g.findGenFuncNameInStmts(d.Cases[j].Body, tempID); r != "unknown_gen" {
					return r
				}
			}
		case LStmtTypeSwitch:
			d := s.Data.(*LTypeSwitch)
			for j := range d.Cases {
				if r := g.findGenFuncNameInStmts(d.Cases[j].Body, tempID); r != "unknown_gen" {
					return r
				}
			}
		}
	}
	return "unknown_gen"
}

// extractFuncNameFromExpr gets the function name from a call expression.
func (g *cGen) extractFuncNameFromExpr(e *LExpr) string {
	if e.Kind == LExprCall {
		d := e.Data.(*LCallData)
		return cSafeName(d.Func)
	}
	return "unknown_gen"
}

// isGenFuncByName checks if a function name corresponds to a generator function.
func (g *cGen) isGenFuncByName(name string) bool {
	for i := range g.prog.Functions {
		f := &g.prog.Functions[i]
		if g.funcName(f) == name && g.isGeneratorFunc(f) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Statement emission
// ---------------------------------------------------------------------------

func (g *cGen) emitStmts(stmts []LStmt) {
	for i := range stmts {
		g.emitStmt(&stmts[i])
	}
}

func (g *cGen) emitStmt(s *LStmt) {
	switch s.Kind {
	case LStmtTempDef:
		d := s.Data.(*LTempDef)
		ty := g.inferExprType(&d.Expr)
		g.tempTypes[d.ID] = ty
		// void-returning expressions can't be assigned to a temp in C
		if ty.Kind == LTyUnit {
			g.linef("%s;", g.emitExprStr(&d.Expr))
			return
		}
		tempName := fmt.Sprintf("_t%d", d.ID)
		g.linef("%s = %s;", g.cFieldDecl(ty, tempName), g.emitExprStr(&d.Expr))

	case LStmtVarDecl:
		d := s.Data.(*LVarDecl)
		if d.Name == "_" {
			if d.Init != nil {
				g.linef("(void)%s;", g.emitValue(d.Init))
			}
			return
		}
		name := cSafeName(d.Name)

		// In generator mode, vars live in the state struct — emit assignment, not declaration
		if g.inGenerator {
			g.varTypes[d.Name] = d.Type
			if d.Init != nil {
				g.linef("_gen->%s = %s;", name, g.emitValue(d.Init))
			}
			return
		}

		varType := d.Type
		// If type contains type vars (from generic code), try to use init value's resolved type
		if d.Init != nil && (varType == nil || g.containsTypeVar(varType) || varType.Kind == LTyAny) {
			if d.Init.Kind == LValTemp {
				if resolved, ok := g.tempTypes[d.Init.TempID]; ok && resolved != nil {
					varType = resolved
				}
			} else if d.Init.Kind == LValVar {
				if resolved, ok := g.varTypes[d.Init.Name]; ok && resolved != nil {
					varType = resolved
				}
			} else if d.Init.Type != nil && !g.containsTypeVar(d.Init.Type) {
				varType = d.Init.Type
			}
		}
		// Store resolved type for later lookups
		g.varTypes[d.Name] = varType
		if d.Init != nil {
			initStr := g.emitValue(d.Init)
			// Wrap in union constructor if target is ForgeUnion and source isn't
			srcType := d.Init.Type
			if varType != nil && varType.Kind == LTyUnion {
				if srcType == nil {
					srcType = g.inferLValType(d.Init)
				}
				if srcType != nil && srcType.Kind != LTyUnion {
					initStr = g.cWrapUnion(initStr, srcType)
				}
			}
			// Wrap in optional if target is optional and source is a bare literal value
			if varType != nil && varType.Kind == LTyOptional {
				if d.Init.Kind == LValLitInt || d.Init.Kind == LValLitUint || d.Init.Kind == LValLitFloat ||
					d.Init.Kind == LValLitString || d.Init.Kind == LValLitBool {
					initStr = fmt.Sprintf("forge_some(%s, %s)", initStr, g.cType(varType))
				}
			}
			g.linef("%s = %s;", g.cFieldDecl(varType, name), initStr)
		} else {
			g.linef("%s = %s;", g.cFieldDecl(varType, name), g.zeroValue(varType))
		}

	case LStmtAssign:
		d := s.Data.(*LAssign)
		target := cSafeName(d.Target)
		if g.inGenerator {
			target = "_gen->" + target
		} else if g.spawnCaptures != nil && g.spawnCaptures[d.Target] {
			target = fmt.Sprintf("(*_ctx->%s)", target)
		} else if g.mutParams[d.Target] {
			target = fmt.Sprintf("(*%s)", target)
		}
		g.linef("%s = %s;", target, g.emitValue(&d.Value))

	case LStmtStructSet:
		d := s.Data.(*LStructSet)
		g.linef("%s.%s = %s;", g.emitValue(&d.Receiver), d.Field, g.emitValue(&d.Value))

	case LStmtClassSet:
		d := s.Data.(*LClassSet)
		g.linef("%s->%s = %s;", g.emitValue(&d.Handle), d.Field, g.emitValue(&d.Value))

	case LStmtIndexSet:
		d := s.Data.(*LIndexSet)
		if d.Collection.Type != nil && d.Collection.Type.Kind == LTyMap {
			g.linef("/* map set not implemented */")
		} else if d.Field != "" {
			g.linef("%s.data[%s].%s = %s;", g.emitValue(&d.Collection), g.emitValue(&d.Index), d.Field, g.emitValue(&d.Value))
		} else {
			g.linef("%s.data[%s] = %s;", g.emitValue(&d.Collection), g.emitValue(&d.Index), g.emitValue(&d.Value))
		}

	case LStmtReturn:
		d := s.Data.(*LReturn)
		if g.inGenerator {
			g.line("return false;")
			return
		}
		if len(d.Values) == 0 {
			g.line("return;")
		} else if len(d.Values) == 1 {
			val := g.emitValue(&d.Values[0])
			// Check if function returns ErrorResult — wrap plain values
			if g.currentFunc != nil && g.currentFunc.ReturnType != nil && g.currentFunc.ReturnType.Kind == LTyErrorResult {
				resultName := g.resultTypeName(g.currentFunc.ReturnType.Elem)
				// If the value is already a result (from MakeResult expr), don't double-wrap
				valType := d.Values[0].Type
				if valType == nil && d.Values[0].Kind == LValTemp {
					valType = g.tempTypes[d.Values[0].TempID]
				}
				if valType == nil && d.Values[0].Kind == LValVar {
					valType = g.varTypes[d.Values[0].Name]
				}
				if valType != nil && valType.Kind == LTyErrorResult {
					g.linef("return %s;", val)
				} else {
					// If the ErrorResult wraps an Optional, and the value isn't already optional, wrap it
					retElem := g.currentFunc.ReturnType.Elem

					if retElem != nil && retElem.Kind == LTyOptional && !g.isClassOptional(retElem) &&
						(valType == nil || valType.Kind != LTyOptional) &&
						d.Values[0].Kind != LValLitNull {
						optName := g.optTypeName(retElem.Elem)
						g.linef("return forge_ok(forge_some(%s, %s), %s);", val, optName, resultName)
					} else {
						g.linef("return forge_ok(%s, %s);", val, resultName)
					}
				}
			} else if g.currentFunc != nil && g.currentFunc.ReturnType != nil && g.currentFunc.ReturnType.Kind == LTyOptional {
				if g.isClassOptional(g.currentFunc.ReturnType) {
					// Class handle optional — just return the pointer (NULL = none)
					if d.Values[0].Kind == LValLitNull {
						g.linef("return NULL;")
					} else {
						g.linef("return %s;", val)
					}
				} else {
					optName := g.optTypeName(g.currentFunc.ReturnType.Elem)
					if d.Values[0].Kind == LValLitNull {
						g.linef("return forge_none(%s);", optName)
					} else if d.Values[0].Type != nil && d.Values[0].Type.Kind == LTyOptional {
						g.linef("return %s;", val)
					} else {
						g.linef("return forge_some(%s, %s);", val, optName)
					}
				}
			} else {
				g.linef("return %s;", val)
			}
		} else if len(d.Values) == 2 && g.currentFunc != nil && g.currentFunc.ReturnType != nil && g.currentFunc.ReturnType.Kind == LTyErrorResult {
			// (value, error) pair return
			resultName := g.resultTypeName(g.currentFunc.ReturnType.Elem)
			valStr := g.emitValue(&d.Values[0])
			// If error is not nil/null, it's an error return
			if d.Values[1].Kind == LValLitNull || d.Values[1].Kind == LValLitString && d.Values[1].StrVal == "" {
				// Wrap value in forge_some if ErrorResult's Elem is Optional and value isn't already optional
				retElem := g.currentFunc.ReturnType.Elem
				valType := d.Values[0].Type
				if valType == nil && d.Values[0].Kind == LValTemp {
					valType = g.tempTypes[d.Values[0].TempID]
				}
				if retElem != nil && retElem.Kind == LTyOptional && !g.isClassOptional(retElem) &&
					(valType == nil || valType.Kind != LTyOptional) &&
					d.Values[0].Kind != LValLitNull {
					optName := g.optTypeName(retElem.Elem)
					g.linef("return forge_ok(forge_some(%s, %s), %s);", valStr, optName, resultName)
				} else {
					g.linef("return forge_ok(%s, %s);", valStr, resultName)
				}
			} else {
				g.linef("return forge_err(%s, %s);", g.emitValueAsCStr(&d.Values[1]), resultName)
			}
		} else {
			// Multi-value return for tuple types — construct tuple struct
			if g.currentFunc != nil && g.currentFunc.ReturnType != nil && g.currentFunc.ReturnType.Kind == LTyTuple {
				tupleName := g.cTupleType(g.currentFunc.ReturnType)
				var fields []string
				for i, v := range d.Values {
					fields = append(fields, fmt.Sprintf("._%d = %s", i, g.emitValue(&v)))
				}
				g.linef("return (%s){ %s };", tupleName, strings.Join(fields, ", "))
			} else {
				g.linef("return %s;", g.emitValue(&d.Values[0]))
			}
		}

	case LStmtIf:
		d := s.Data.(*LIf)
		g.linef("if (%s) {", g.emitValue(&d.Cond))
		g.indent++
		g.emitStmts(d.Then)
		g.indent--
		if len(d.Else) > 0 {
			g.line("} else {")
			g.indent++
			g.emitStmts(d.Else)
			g.indent--
		}
		g.line("}")

	case LStmtWhile:
		d := s.Data.(*LWhile)
		g.line("while (1) {")
		g.indent++
		g.emitStmts(d.CondBlock)
		g.linef("if (!(%s)) break;", g.emitValue(&d.CondVar))
		g.emitStmts(d.Body)
		g.indent--
		g.line("}")

	case LStmtFor:
		d := s.Data.(*LFor)
		collStr := g.emitValue(&d.Collection)

		// Generator iteration: init + loop calling next
		if d.Collection.Type != nil && d.Collection.Type.Kind == LTyGenerator {
			// The collection is a call to a generator function.
			// collStr is e.g. "count_up_init(5)" — but actually the lowerer
			// emits it as a function call expression that returns gen T.
			// We need to figure out the generator function name from the call.
			// The collection value should be a temp whose expr is a function call.
			genBaseName := g.resolveGenBaseName(&d.Collection)
			structName := genBaseName + "_gen_t"
			iterVar := fmt.Sprintf("_gen_iter_%d", g.lambdaID)
			g.lambdaID++
			g.linef("%s* %s = %s;", structName, iterVar, collStr)
			g.linef("while (%s_next(%s)) {", genBaseName, iterVar)
			g.indent++
			g.linef("%s %s = %s->_value;", g.cType(d.VarType), d.Var, iterVar)
			g.emitStmts(d.Body)
			g.indent--
			g.line("}")
			g.linef("free(%s);", iterVar)
			break
		}

		if d.IndexVar != "" {
			g.linef("for (int32_t %s = 0; %s < %s.len; %s++) {", d.IndexVar, d.IndexVar, collStr, d.IndexVar)
			g.indent++
			g.linef("%s %s = %s.data[%s];", g.cType(d.VarType), d.Var, collStr, d.IndexVar)
		} else {
			idxVar := "_idx"
			g.linef("for (int32_t %s = 0; %s < %s.len; %s++) {", idxVar, idxVar, collStr, idxVar)
			g.indent++
			g.linef("%s %s = %s.data[%s];", g.cType(d.VarType), d.Var, collStr, idxVar)
		}
		g.emitStmts(d.Body)
		g.indent--
		g.line("}")

	case LStmtSwitch:
		d := s.Data.(*LSwitch)
		g.linef("switch (%s) {", g.emitValue(&d.Tag))
		for _, c := range d.Cases {
			if c.Tag == -1 {
				g.line("default: {")
			} else {
				g.linef("case %d: {", c.Tag)
			}
			g.indent++
			if c.Binding != "" && d.EnumName != "" {
				enumName := g.structName(d.EnumName, false)
				variant := c.Binding
				variantLower := cSafeName(strings.ToLower(variant))
				// Extract variant data and bind to name
				g.linef("%s_%s_Data %s = %s.data.%s;",
					enumName, variant, c.Binding,
					g.emitValue(&d.Tag), variantLower)
			}
			g.emitStmts(c.Body)
			g.line("break;")
			g.indent--
			g.line("}")
		}
		// If no default case exists, add __builtin_unreachable() to silence
		// GCC "control reaches end of non-void function" warnings on exhaustive switches.
		hasDefault := false
		for _, c := range d.Cases {
			if c.Tag == -1 {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			g.line("default: __builtin_unreachable();")
		}
		g.line("}")

	case LStmtTypeSwitch:
		d := s.Data.(*LTypeSwitch)
		g.emitTypeSwitch(d)

	case LStmtBlock:
		d := s.Data.(*LBlock)
		g.line("{")
		g.indent++
		g.emitStmts(d.Stmts)
		g.indent--
		g.line("}")

	case LStmtSideEffect:
		d := s.Data.(*LSideEffect)
		expr := g.emitSideEffect(&d.Expr)
		if expr != "" {
			g.linef("%s;", expr)
		}

	case LStmtMultiAssign:
		d := s.Data.(*LMultiAssign)
		g.emitMultiAssign(d)

	case LStmtSend:
		d := s.Data.(*LSend)
		chanType := d.Channel.Type
		if chanType == nil {
			if d.Channel.Kind == LValVar {
				chanType = g.varTypes[d.Channel.Name]
			} else if d.Channel.Kind == LValTemp {
				if t, ok := g.tempTypes[d.Channel.TempID]; ok {
					chanType = t
				}
			}
		}
		suffix := "void"
		if chanType != nil && chanType.Kind == LTyChannel && chanType.Elem != nil {
			suffix = g.chanSuffix(chanType.Elem)
		}
		g.linef("forge_chan_send_%s(%s, %s);", suffix, g.emitValue(&d.Channel), g.emitValue(&d.Value))

	case LStmtSpawn:
		d := s.Data.(*LSpawn)
		g.spawnID++
		funcName := fmt.Sprintf("_spawn_%d", g.spawnID)

		// Auto-detect captured variables: used in body but not declared within it
		usedVars := collectUsedVars(d.Body)
		declaredVars := collectDeclaredVars(d.Body)
		var captures []cCapture
		for varName := range usedVars {
			if !declaredVars[varName] {
				ctyp := "void*"
				if t, ok := g.varTypes[varName]; ok && t != nil {
					ctyp = g.cType(t)
				}
				captures = append(captures, cCapture{name: varName, typ: ctyp})
			}
		}

		// Emit the body into a separate buffer with capture awareness
		savedBuf := g.buf
		savedIndent := g.indent
		savedCaptures := g.spawnCaptures
		g.buf = strings.Builder{}
		g.indent = 1
		// Set spawn captures so emitValue redirects variable references to _ctx->
		captureSet := map[string]bool{}
		for _, c := range captures {
			captureSet[c.name] = true
		}
		g.spawnCaptures = captureSet
		g.emitStmts(d.Body)
		bodyStr := g.buf.String()
		g.buf = savedBuf
		g.indent = savedIndent
		g.spawnCaptures = savedCaptures

		g.spawnFuncs = append(g.spawnFuncs, cSpawnFunc{
			name:     funcName,
			bodyStr:  bodyStr,
			captures: captures,
		})

		// Emit spawn call — pass pointers to original variables (Go-style capture-by-reference)
		if len(captures) > 0 {
			g.linef("{")
			g.indent++
			g.linef("%s_ctx* _ctx = (%s_ctx*)malloc(sizeof(%s_ctx));", funcName, funcName, funcName)
			for _, c := range captures {
				g.linef("_ctx->%s = &%s;", c.name, c.name)
			}
			g.linef("forge_spawn(%s, _ctx);", funcName)
			g.indent--
			g.linef("}")
		} else {
			g.linef("forge_spawn(%s, NULL);", funcName)
		}

	case LStmtSelect:
		g.line("/* select: channels not supported in C backend */")

	case LStmtDefer:
		d := s.Data.(*LDefer)
		g.line("/* defer (executed inline — no stack unwinding): */")
		g.emitStmts(d.Body)

	case LStmtLock:
		d := s.Data.(*LLock)
		mutexVal := g.emitValue(&d.Mutex)
		g.linef("pthread_mutex_lock(&%s);", mutexVal)
		g.emitStmts(d.Body)
		g.linef("pthread_mutex_unlock(&%s);", mutexVal)

	case LStmtExpr:
		d := s.Data.(*LExprStmt)
		// Skip references to void temps (e.g., println return)
		if ty, ok := g.tempTypes[d.TempID]; ok && ty.Kind == LTyUnit {
			return
		}
		g.linef("_t%d;", d.TempID)

	case LStmtBreak:
		g.line("break;")

	case LStmtContinue:
		g.line("continue;")

	case LStmtYield:
		val := s.Data.(LValue)
		g.genYieldCount++
		stateNum := g.genYieldCount
		g.linef("_gen->_value = %s;", g.emitValue(&val))
		g.linef("_gen->_state = %d;", stateNum)
		g.linef("return true;")
		g.linef("_gen_s%d:;", stateNum)
	}
}

// emitTypeSwitch handles type switches for both enum tagged unions and ad-hoc unions.
func (g *cGen) emitTypeSwitch(d *LTypeSwitch) {
	val := g.emitValue(&d.Value)

	// Check if this is an ad-hoc union (LTyUnion) type switch
	if d.Value.Type != nil && d.Value.Type.Kind == LTyUnion {
		g.linef("switch (%s.tag) {", val)
		for _, c := range d.Cases {
			if c.Type != nil {
				tag := g.unionTagForType(c.Type)
				g.linef("case %s: {", tag)
			} else {
				g.line("default: {")
			}
			g.indent++
			g.emitStmts(c.Body)
			g.line("break;")
			g.indent--
			g.line("}")
		}
		g.line("}")
		return
	}

	g.linef("switch (%s.tag) {", val)
	for _, c := range d.Cases {
		if c.Type != nil {
			tagConst := g.resolveTagConstant(d, c.Type)
			if tagConst != "" {
				g.linef("case %s: {", tagConst)
			} else {
				g.linef("case /* %s */0: {", g.cType(c.Type))
			}
		} else {
			g.line("default: {")
		}
		g.indent++
		g.emitStmts(c.Body)
		g.line("break;")
		g.indent--
		g.line("}")
	}
	g.line("}")
}

// resolveTagConstant finds the tag constant for a variant type in a type switch.
func (g *cGen) resolveTagConstant(d *LTypeSwitch, caseType *LType) string {
	// The value being switched on should be a tagged union
	valType := d.Value.Type
	if valType == nil {
		return ""
	}
	enumName := g.structName(valType.Name, valType.IsExported)

	// For tagged unions, the case type's name is the variant name
	variantName := ""
	if caseType != nil && caseType.Name != "" {
		// The case type name might be "EnumName_Variant_Data" or just "Variant"
		variantName = caseType.Name
		// Strip the enum prefix if present
		prefix := enumName + "_"
		if strings.HasPrefix(variantName, prefix) {
			variantName = strings.TrimPrefix(variantName, prefix)
			variantName = strings.TrimSuffix(variantName, "_Data")
		}
	}

	if variantName == "" {
		return ""
	}

	// Look up in program enums to verify
	for _, e := range g.prog.Enums {
		eName := g.structName(e.Name, e.IsExported)
		if eName == enumName {
			for _, v := range e.Variants {
				if v.Name == variantName {
					return fmt.Sprintf("%s_%s", enumName, v.Name)
				}
			}
		}
	}
	// Fallback: construct it
	return fmt.Sprintf("%s_%s", enumName, variantName)
}

// unionTagForType returns the FORGE_UNION_TAG_* constant for a given LType.
func (g *cGen) unionTagForType(t *LType) string {
	switch t.Kind {
	case LTyI8, LTyI16, LTyI32, LTyU8, LTyU16, LTyU32, LTyPlatformInt:
		return "FORGE_UNION_TAG_I32"
	case LTyI64, LTyU64, LTyPlatformUint:
		return "FORGE_UNION_TAG_I64"
	case LTyF32:
		return "FORGE_UNION_TAG_F32"
	case LTyF64:
		return "FORGE_UNION_TAG_F64"
	case LTyBool:
		return "FORGE_UNION_TAG_BOOL"
	case LTyString:
		return "FORGE_UNION_TAG_STRING"
	default:
		return "FORGE_UNION_TAG_PTR"
	}
}

// cWrapUnion wraps a C expression string in a forge_union_* constructor based on source type.
func (g *cGen) cWrapUnion(expr string, srcType *LType) string {
	if srcType == nil {
		return fmt.Sprintf("forge_union_ptr((void*)(%s))", expr)
	}
	switch srcType.Kind {
	case LTyI8, LTyI16, LTyI32, LTyU8, LTyU16, LTyU32, LTyPlatformInt:
		return fmt.Sprintf("forge_union_i32((int32_t)(%s))", expr)
	case LTyI64, LTyU64, LTyPlatformUint:
		return fmt.Sprintf("forge_union_i64((int64_t)(%s))", expr)
	case LTyF32:
		return fmt.Sprintf("forge_union_f32(%s)", expr)
	case LTyF64:
		return fmt.Sprintf("forge_union_f64(%s)", expr)
	case LTyBool:
		return fmt.Sprintf("forge_union_bool(%s)", expr)
	case LTyString:
		return fmt.Sprintf("forge_union_string(%s)", expr)
	default:
		return fmt.Sprintf("forge_union_ptr((void*)(%s))", expr)
	}
}

// inferLValType infers the type of an LValue from its literal kind when Type is nil.
func (g *cGen) inferLValType(v *LValue) *LType {
	if v.Type != nil {
		return v.Type
	}
	switch v.Kind {
	case LValLitInt:
		return &LType{Kind: LTyI32, Bits: 32}
	case LValLitUint:
		return &LType{Kind: LTyU32, Bits: 32}
	case LValLitFloat:
		return &LType{Kind: LTyF64, Bits: 64}
	case LValLitString:
		return &LType{Kind: LTyString}
	case LValLitBool:
		return &LType{Kind: LTyBool}
	case LValTemp:
		if t, ok := g.tempTypes[v.TempID]; ok {
			return t
		}
	case LValVar:
		if t, ok := g.varTypes[v.Name]; ok {
			return t
		}
	}
	return nil
}

// emitSideEffect handles side-effect expressions (append, method calls, etc.)
// Returns the expression string, or "" if it was handled via multi-line emission.
func (g *cGen) emitSideEffect(e *LExpr) string {
	// Special case: append as side effect needs to reassign to the slice
	if e.Kind == LExprBuiltin {
		d := e.Data.(*LBuiltinData)
		if (d.Name == "append" || d.Name == "slice_push") && len(d.Args) >= 2 {
			sliceArg := g.emitValue(&d.Args[0])
			elemArg := g.emitValue(&d.Args[1])
			sliceType := g.sliceTypeNameFromValue(&d.Args[0])
			// forge_push is a macro that takes a pointer to the slice
			g.linef("forge_push(&%s, %s, %s);", sliceArg, elemArg, sliceType)
			return ""
		}
		if d.Name == "push_bytes" && len(d.Args) >= 2 {
			dstArg := g.emitValue(&d.Args[0])
			srcArg := g.emitValue(&d.Args[1])
			g.linef("forge_push_bytes(&%s, %s);", dstArg, srcArg)
			return ""
		}
	}
	return g.emitExprStr(e)
}

func (g *cGen) sliceTypeNameFromValue(v *LValue) string {
	if v.Type != nil && v.Type.Kind == LTySlice {
		return g.sliceTypeName(v.Type.Elem)
	}
	return "/* unknown slice type */"
}

// emitMultiAssign handles (val, err) = expr patterns
func (g *cGen) emitMultiAssign(d *LMultiAssign) {
	exprStr := g.emitExprStr(&d.Expr)
	// Detect error result pattern: 2 names, second is error type
	isErrorResult := d.Expr.Type != nil && d.Expr.Type.Kind == LTyErrorResult
	// Also detect when MultiAssign types indicate (T, error) pattern
	if !isErrorResult && len(d.Types) == 2 && d.Types[1] != nil && (d.Types[1].Kind == LTyError || d.Types[1].Kind == LTyString) {
		isErrorResult = true
	}
	// Also check if the call is to a function that returns ErrorResult
	if !isErrorResult && d.Expr.Kind == LExprCall {
		cd := d.Expr.Data.(*LCallData)
		for _, fn := range g.prog.Functions {
			if g.funcName(&fn) == cd.Func && fn.ReturnType != nil && fn.ReturnType.Kind == LTyErrorResult {
				isErrorResult = true
				// Also capture the element type from the function's return type
				if fn.ReturnType.Elem != nil {
					d.Expr.Type = fn.ReturnType
				}
				break
			}
		}
	}
	if len(d.Names) == 2 && isErrorResult {
		// Error result destructuring
		var elemType *LType
		if len(d.Types) > 0 {
			elemType = d.Types[0]
		}
		if d.Expr.Type != nil && d.Expr.Type.Elem != nil {
			elemType = d.Expr.Type.Elem
		}
		if elemType == nil {
			elemType = &LType{Kind: LTyI32}
		}
		resultType := g.resultTypeName(elemType)
		tmpName := fmt.Sprintf("_multi_%d", g.nextTemp())
		g.linef("%s %s = %s;", resultType, tmpName, exprStr)
		if d.Names[0] != "_" {
			valType := g.cType(elemType)
			g.linef("%s %s = %s.value;", valType, d.Names[0], tmpName)
			g.varTypes[d.Names[0]] = elemType
		}
		if d.Names[1] != "_" {
			g.linef("const char* %s = %s.error;", d.Names[1], tmpName)
			g.varTypes[d.Names[1]] = &LType{Kind: LTyError}
		}
	} else if len(d.Names) == 2 && d.Expr.Type != nil && d.Expr.Type.Kind == LTyTuple {
		tmpName := fmt.Sprintf("_multi_%d", g.nextTemp())
		tupleType := g.cTupleType(d.Expr.Type)
		g.linef("%s %s = %s;", tupleType, tmpName, exprStr)
		for i, name := range d.Names {
			if name != "_" {
				fieldType := "int"
				var ltyp *LType
				if i < len(d.Expr.Type.Fields) {
					ltyp = d.Expr.Type.Fields[i].Type
					fieldType = g.cType(ltyp)
				}
				g.linef("%s %s = %s._%d;", fieldType, name, tmpName, i)
				if ltyp != nil {
					g.varTypes[name] = ltyp
				}
			}
		}
	} else {
		g.linef("/* multi-assign: %s = %s */", strings.Join(d.Names, ", "), exprStr)
	}
}

// ---------------------------------------------------------------------------
// Expression emission (returns string)
// ---------------------------------------------------------------------------

func (g *cGen) emitExprStr(e *LExpr) string {
	switch e.Kind {
	case LExprCall:
		d := e.Data.(*LCallData)
		name := cSafeName(d.Func)
		// Map known stdlib functions
		if name == "fmt.Println" || name == "Println" || name == "println" {
			return g.emitPrintln(d.Args)
		}
		if name == "fmt.Printf" || name == "Printf" || name == "printf" {
			return g.emitPrintf(d.Args)
		}
		if name == "fmt.Sprintf" || name == "Sprintf" {
			return g.emitSprintf(d.Args)
		}
		if name == "fmt.Errorf" || name == "errors.New" {
			if len(d.Args) > 0 {
				return g.emitValueAsCStr(&d.Args[0])
			}
			return `"error"`
		}
		if name == "panic" && len(d.Args) > 0 {
			return fmt.Sprintf("forge_panic(%s)", g.emitValue(&d.Args[0]))
		}
		// strconv functions
		if name == "strconv.Itoa" && len(d.Args) > 0 {
			return fmt.Sprintf("forge_sprintf(\"%%d\", %s)", g.emitValue(&d.Args[0]))
		}
		if name == "strconv.Atoi" && len(d.Args) > 0 {
			resultType := g.resultTypeName(&LType{Kind: LTyI32})
			return fmt.Sprintf("({ int _v = atoi(%s); %s _r = forge_ok(_v, %s); _r; })",
				g.emitValueAsCStr(&d.Args[0]), resultType, resultType)
		}
		// strings stdlib
		if name == "strings.ToUpper" && len(d.Args) > 0 {
			return fmt.Sprintf("forge_toupper(%s)", g.emitValue(&d.Args[0]))
		}
		if name == "strings.ToLower" && len(d.Args) > 0 {
			return fmt.Sprintf("forge_tolower(%s)", g.emitValue(&d.Args[0]))
		}
		if name == "strings.Contains" && len(d.Args) >= 2 {
			return fmt.Sprintf("(strstr(%s, %s) != NULL)", g.emitValue(&d.Args[0]), g.emitValue(&d.Args[1]))
		}
		args := g.emitArgsBoxed(d.Func, d.Args, d.MutArgs)
		// If calling a generator function, redirect to its _init function
		if g.isGenFuncByName(name) {
			return fmt.Sprintf("%s_init(%s)", name, args)
		}
		// Map Forge builtins to runtime functions
		switch name {
		case "atoi":
			return fmt.Sprintf("forge_atoi(%s)", args)
		case "parse_float":
			return fmt.Sprintf("forge_parse_float(%s)", args)
		case "itoa":
			return fmt.Sprintf("forge_itoa(%s)", args)
		}
		return fmt.Sprintf("%s(%s)", name, args)

	case LExprMethodCall:
		d := e.Data.(*LMethodCallData)
		recv := g.emitValue(&d.Receiver)
		args := g.emitArgs(d.Args, d.MutArgs)

		// Check if receiver is interface-typed → vtable dispatch
		recvType := d.Receiver.Type
		if recvType == nil {
			if d.Receiver.Kind == LValTemp {
				recvType = g.tempTypes[d.Receiver.TempID]
			} else if d.Receiver.Kind == LValVar {
				recvType = g.varTypes[d.Receiver.Name]
			}
		}
		if recvType != nil && recvType.Kind == LTyAny && recvType.Name != "" {
			if _, isIface := g.ifaceByName[recvType.Name]; isIface {
				// Vtable dispatch: recv._vtable->method(recv._data, args...)
				if args != "" {
					return fmt.Sprintf("%s._vtable->%s(%s._data, %s)", recv, d.Method, recv, args)
				}
				return fmt.Sprintf("%s._vtable->%s(%s._data)", recv, d.Method, recv)
			}
		}

		className := ""
		if d.Receiver.Type != nil {
			className = d.Receiver.Type.Name
			// For Optional(ClassHandle), extract the class name
			if d.Receiver.Type.Kind == LTyOptional && d.Receiver.Type.Elem != nil && d.Receiver.Type.Elem.Kind == LTyClassHandle {
				className = d.Receiver.Type.Elem.Name
			}
		}
		if g.prog.ClassRenames != nil {
			if renamed, ok := g.prog.ClassRenames[className]; ok {
				className = renamed
			}
		}
		// If className is still empty (type var, any), try to resolve from temp/var types
		if className == "" {
			var resolved *LType
			if d.Receiver.Kind == LValTemp {
				resolved = g.tempTypes[d.Receiver.TempID]
			} else if d.Receiver.Kind == LValVar {
				resolved = g.varTypes[d.Receiver.Name]
			}
			if resolved != nil {
				className = resolved.Name
				if resolved.Kind == LTyOptional && resolved.Elem != nil {
					className = resolved.Elem.Name
				}
			}
		}
		// If still empty, search program for a matching method
		if className == "" {
			for _, fn := range g.prog.Functions {
				if fn.Receiver != "" && fn.Name == d.Method {
					className = fn.Receiver
					break
				}
			}
		}
		methodName := g.structName(className, false) + "_" + d.Method
		if args != "" {
			return fmt.Sprintf("%s(%s, %s)", methodName, recv, args)
		}
		return fmt.Sprintf("%s(%s)", methodName, recv)

	case LExprClassAlloc:
		d := e.Data.(*LClassAllocData)
		name := g.structName(d.Class, false)
		// Look up class fields for optional wrapping
		var classFields []LField
		for _, c := range g.prog.Classes {
			if c.Name == d.Class {
				classFields = c.Fields
				break
			}
		}
		var fieldInits []string
		for _, f := range d.Fields {
			val := g.emitValue(&f.Value)
			// Auto-wrap non-null values for optional fields
			if fieldType := g.findClassField(classFields, f.Name); fieldType != nil && fieldType.Kind == LTyOptional {
				// Class handle optionals are just pointers — no ForgeOpt wrapping needed
				if fieldType.Elem != nil && fieldType.Elem.Kind == LTyClassHandle {
					if f.Value.Kind == LValLitNull {
						val = "NULL"
					}
					// else: value is already a pointer, use as-is
				} else if f.Value.Kind == LValLitNull {
					optName := g.optTypeName(fieldType.Elem)
					val = fmt.Sprintf("forge_none(%s)", optName)
				} else if f.Value.Type == nil || f.Value.Type.Kind != LTyOptional {
					optName := g.optTypeName(fieldType.Elem)
					val = fmt.Sprintf("forge_some(%s, %s)", val, optName)
				}
			}
			fieldInits = append(fieldInits, fmt.Sprintf(".%s = %s", lcFirst(f.Name), val))
		}
		return fmt.Sprintf("({ %s* _p = malloc(sizeof(%s)); *_p = (%s){%s}; _p; })",
			name, name, name, strings.Join(fieldInits, ", "))

	case LExprStructLit:
		d := e.Data.(*LStructLitData)
		// Check if this is actually a slice literal (lowerer emits struct lit for slice)
		if e.Type != nil && e.Type.Kind == LTySlice {
			sliceType := g.sliceTypeName(e.Type.Elem)
			elemType := g.cType(e.Type.Elem)
			if len(d.Fields) == 0 {
				return fmt.Sprintf("forge_slice_empty(%s)", sliceType)
			}
			var elems []string
			for _, f := range d.Fields {
				elems = append(elems, g.emitValue(&f.Value))
			}
			return fmt.Sprintf("forge_slice_lit(%s, %s, %s)", sliceType, elemType, strings.Join(elems, ", "))
		}
		var fieldInits []string
		isClass := e.Type != nil && e.Type.Kind == LTyClassHandle
		for _, f := range d.Fields {
			fname := f.Name
			if isClass {
				fname = lcFirst(fname)
			}
			if fname == "" {
				// Positional field — no designator
				fieldInits = append(fieldInits, g.emitValue(&f.Value))
			} else {
				fieldInits = append(fieldInits, fmt.Sprintf(".%s = %s", fname, g.emitValue(&f.Value)))
			}
		}
		// Class handles are heap-allocated — use malloc pattern
		if isClass {
			name := g.structName(e.Type.Name, false)
			return fmt.Sprintf("({ %s* _p = malloc(sizeof(%s)); *_p = (%s){%s}; _p; })",
				name, name, name, strings.Join(fieldInits, ", "))
		}
		// Mutex types use PTHREAD_MUTEX_INITIALIZER
		if e.Type != nil && e.Type.Kind == LTyMutex {
			return "(pthread_mutex_t)PTHREAD_MUTEX_INITIALIZER"
		}
		typeName := "/* struct */"
		if e.Type != nil {
			typeName = g.cType(e.Type)
		}
		return fmt.Sprintf("(%s){%s}", typeName, strings.Join(fieldInits, ", "))

	case LExprBinOp:
		d := e.Data.(*LBinOpData)
		left := g.emitValue(&d.Left)
		right := g.emitValue(&d.Right)
		// String equality
		if d.Left.Type != nil && d.Left.Type.Kind == LTyString {
			if d.Op == LBinEq {
				return fmt.Sprintf("forge_str_eq(%s, %s)", left, right)
			}
			if d.Op == LBinNe {
				return fmt.Sprintf("(!forge_str_eq(%s, %s))", left, right)
			}
			if d.Op == LBinAdd {
				return fmt.Sprintf("forge_str_concat(%s, %s)", left, right)
			}
		}
		// Optional null comparison: opt != null → opt.has_value, opt == null → !opt.has_value
		// Only for non-pointer optionals (class handle optionals use NULL pointer)
		if (d.Op == LBinEq || d.Op == LBinNe) &&
			(d.Right.Kind == LValLitNull || d.Left.Kind == LValLitNull) {
			var optVal string
			var optType *LType
			if d.Right.Kind == LValLitNull {
				optVal = left
				optType = d.Left.Type
				if optType == nil && d.Left.Kind == LValTemp {
					optType = g.tempTypes[d.Left.TempID]
				} else if optType == nil && d.Left.Kind == LValVar {
					optType = g.varTypes[d.Left.Name]
				}
			} else {
				optVal = right
				optType = d.Right.Type
			}
			if optType != nil && optType.Kind == LTyOptional && !g.isClassOptional(optType) {
				if d.Op == LBinNe {
					return fmt.Sprintf("%s.has", optVal)
				}
				return fmt.Sprintf("(!%s.has)", optVal)
			}
		}
		return fmt.Sprintf("(%s %s %s)", left, cBinOp(d.Op), right)

	case LExprUnOp:
		d := e.Data.(*LUnOpData)
		return fmt.Sprintf("(%s%s)", cUnOp(d.Op), g.emitValue(&d.Operand))

	case LExprCast:
		d := e.Data.(*LCastData)
		return fmt.Sprintf("((%s)%s)", g.cType(d.Target), g.emitValue(&d.Operand))

	case LExprBuiltin:
		d := e.Data.(*LBuiltinData)
		return g.emitBuiltin(d)

	case LExprStructField:
		d := e.Data.(*LStructFieldData)
		// Check if receiver is actually a class handle (pointer) — use -> not .
		recvType := d.Receiver.Type
		if d.Receiver.Kind == LValTemp {
			if resolved, ok := g.tempTypes[d.Receiver.TempID]; ok && resolved != nil {
				recvType = resolved
			}
		} else if d.Receiver.Kind == LValVar {
			if resolved, ok := g.varTypes[d.Receiver.Name]; ok && resolved != nil {
				recvType = resolved
			}
		}
		if recvType != nil && recvType.Kind == LTyClassHandle {
			return fmt.Sprintf("%s->%s", g.emitValue(&d.Receiver), d.Field)
		}
		// Map tuple field access ._0/._1 to .val/.err for ErrorResult types
		if recvType != nil && recvType.Kind == LTyErrorResult {
			field := d.Field
			if field == "_0" {
				field = "value"
			} else if field == "_1" {
				field = "error"
			}
			return fmt.Sprintf("%s.%s", g.emitValue(&d.Receiver), field)
		}
		return fmt.Sprintf("%s.%s", g.emitValue(&d.Receiver), d.Field)

	case LExprClassGet:
		d := e.Data.(*LClassGetData)
		return fmt.Sprintf("%s->%s", g.emitValue(&d.Handle), d.Field)

	case LExprIndexGet:
		d := e.Data.(*LIndexGetData)
		coll := g.emitValue(&d.Collection)
		idx := g.emitValue(&d.Index)
		// String indexing returns a byte — strings are [u8] slices
		if d.Collection.Type != nil && d.Collection.Type.Kind == LTyString {
			return fmt.Sprintf("%s.data[%s]", coll, idx)
		}
		return fmt.Sprintf("%s.data[%s]", coll, idx)

	case LExprSlice:
		d := e.Data.(*LSliceData)
		coll := g.emitValue(&d.Collection)
		low := "0"
		if d.Low != nil {
			low = g.emitValue(d.Low)
		}
		high := fmt.Sprintf("%s.len", coll)
		if d.High != nil {
			high = g.emitValue(d.High)
		}
		sliceType := g.cType(e.Type)
		return fmt.Sprintf("forge_subslice(%s, %s, %s, %s)", coll, low, high, sliceType)

	case LExprWrapOptional:
		d := e.Data.(*LWrapOptionalData)
		// Class handles are already pointers (nullable) — no wrapping needed
		if g.isClassOptional(e.Type) {
			return g.emitValue(&d.Value)
		}
		optName := g.optTypeNameFromExpr(e)
		return fmt.Sprintf("forge_some(%s, %s)", g.emitValue(&d.Value), optName)

	case LExprUnwrapOptional:
		d := e.Data.(*LUnwrapOptionalData)
		valType := d.Value.Type
		if d.Value.Kind == LValTemp {
			if resolved, ok := g.tempTypes[d.Value.TempID]; ok && resolved != nil {
				valType = resolved
			}
		} else if d.Value.Kind == LValVar {
			if resolved, ok := g.varTypes[d.Value.Name]; ok && resolved != nil {
				valType = resolved
			}
		}
		if valType != nil && (valType.Kind == LTyClassHandle || g.isClassOptional(valType)) {
			return g.emitValue(&d.Value)
		}
		return fmt.Sprintf("forge_unwrap(%s)", g.emitValue(&d.Value))

	case LExprIsNull:
		d := e.Data.(*LIsNullData)
		valType := d.Value.Type
		// Try to resolve through temp/var types for better class handle detection
		if d.Value.Kind == LValTemp {
			if resolved, ok := g.tempTypes[d.Value.TempID]; ok && resolved != nil {
				valType = resolved
			}
		} else if d.Value.Kind == LValVar {
			if resolved, ok := g.varTypes[d.Value.Name]; ok && resolved != nil {
				valType = resolved
			}
		}
		if valType != nil && (valType.Kind == LTyClassHandle || valType.Kind == LTyAny ||
			(valType.Kind == LTyOptional && valType.Elem != nil && valType.Elem.Kind == LTyClassHandle)) {
			return fmt.Sprintf("(%s == NULL)", g.emitValue(&d.Value))
		}
		return fmt.Sprintf("forge_isnull(%s)", g.emitValue(&d.Value))

	case LExprVariantConstruct:
		d := e.Data.(*LVariantConstructData)
		enumName := d.Enum
		if enumName == "" {
			enumName = "Enum"
		}
		structName := g.structName(enumName, false)
		// Simple enums (all unit variants) — just the tag constant
		if g.simpleEnums[structName] {
			return fmt.Sprintf("%s_%s", structName, d.Variant)
		}
		if len(d.Fields) == 0 {
			return fmt.Sprintf("(%s){.tag = %s_%s}", structName, structName, d.Variant)
		}
		var fieldInits []string
		for _, f := range d.Fields {
			fieldInits = append(fieldInits, g.emitValue(&f))
		}
		variantLower := cSafeName(strings.ToLower(d.Variant))
		dataInit := strings.Join(fieldInits, ", ")
		return fmt.Sprintf("(%s){.tag = %s_%s, .data.%s = {%s}}",
			structName, structName, d.Variant, variantLower, dataInit)

	case LExprVariantTag:
		d := e.Data.(*LVariantTagData)
		val := g.emitValue(&d.Value)
		// For simple enums, the value IS the tag (no .tag field)
		if vt := g.resolveValueType(&d.Value); vt != nil && vt.Kind == LTyTaggedUnion {
			name := g.structName(vt.Name, false)
			if g.simpleEnums[name] {
				return val
			}
		}
		return fmt.Sprintf("%s.tag", val)

	case LExprVariantData:
		d := e.Data.(*LVariantDataData)
		variantLower := cSafeName(strings.ToLower(d.Variant))
		if d.Field != "" {
			return fmt.Sprintf("%s.data.%s.%s", g.emitValue(&d.Value), variantLower, d.Field)
		}
		return fmt.Sprintf("%s.data.%s", g.emitValue(&d.Value), variantLower)

	case LExprFuncLit:
		d := e.Data.(*LFuncLitData)
		return g.emitFuncLit(d, e.Type)

	case LExprFormat:
		d := e.Data.(*LFormatData)
		return g.emitFormat(d)

	case LExprExtractValue:
		d := e.Data.(*LExtractValueData)
		return fmt.Sprintf("%s.value", g.emitValue(&d.Value))

	case LExprExtractError:
		d := e.Data.(*LExtractErrorData)
		return fmt.Sprintf("%s.error", g.emitValue(&d.Value))

	case LExprMakeResult:
		d := e.Data.(*LMakeResultData)
		resultName := g.resultTypeNameFromExpr(e)
		// If there's an error value, it's an error result; otherwise ok
		if d.Err.Kind == LValLitNull || (d.Err.Kind == LValLitString && d.Err.StrVal == "") {
			return fmt.Sprintf("forge_ok(%s, %s)", g.emitValue(&d.Value), resultName)
		}
		return fmt.Sprintf("forge_err(%s, %s)", g.emitValueAsCStr(&d.Err), resultName)

	case LExprMakeSlice:
		sliceType := g.sliceTypeNameFromType(e.Type)
		return fmt.Sprintf("forge_slice_empty(%s)", sliceType)

	case LExprMakeMap:
		return "NULL /* make_map: not supported in C backend */"

	case LExprMakeChannel:
		d := e.Data.(*LMakeChannelData)
		suffix := g.chanSuffix(d.ElemType)
		bufSize := "0"
		if d.BufSize != nil {
			bufSize = g.emitValue(d.BufSize)
		}
		return fmt.Sprintf("forge_chan_make_%s(%s)", suffix, bufSize)

	case LExprFuncRef:
		d := e.Data.(*LFuncRefData)
		return d.Name

	case LExprEnvGet:
		d := e.Data.(*LEnvGetData)
		return fmt.Sprintf("%s.%s", g.emitValue(&d.Env), d.Field)

	default:
		return fmt.Sprintf("/* unknown expr kind %d */", e.Kind)
	}
}

// emitFuncLit emits a function literal (lambda).
// Lambdas are hoisted to top-level static functions; this returns just the function name.
func (g *cGen) emitFuncLit(d *LFuncLitData, typ *LType) string {
	var params []string
	for _, p := range d.Params {
		params = append(params, g.cFieldDecl(p.Type, p.Name))
	}
	retType := "void"
	if typ != nil && typ.Kind == LTyFuncPtr && typ.Return != nil {
		retType = g.cType(typ.Return)
	} else if d.ReturnType != nil {
		retType = g.cType(d.ReturnType)
	}

	g.lambdaID++
	lambdaName := fmt.Sprintf("_lambda_%d", g.lambdaID)
	paramStr := strings.Join(params, ", ")
	if paramStr == "" {
		paramStr = "void"
	}

	// Emit body into temporary buffer
	var bodyBuf strings.Builder
	oldBuf := g.buf
	g.buf = bodyBuf
	g.indent++
	g.emitStmts(d.Body)
	g.indent--
	bodyStr := g.buf.String()
	g.buf = oldBuf

	// Store for hoisting
	g.lambdas = append(g.lambdas, cLambda{
		name:    lambdaName,
		retType: retType,
		params:  paramStr,
		bodyStr: bodyStr,
	})

	return lambdaName
}

// ---------------------------------------------------------------------------
// Value emission
// ---------------------------------------------------------------------------

func (g *cGen) emitValue(v *LValue) string {
	switch v.Kind {
	case LValVar:
		name := cSafeName(v.Name)
		if g.inGenerator {
			return "_gen->" + name
		}
		if g.spawnCaptures != nil && g.spawnCaptures[v.Name] {
			return fmt.Sprintf("(*_ctx->%s)", name)
		}
		// mut params are passed as pointers — dereference when reading value
		if g.mutParams[v.Name] {
			return fmt.Sprintf("(*%s)", name)
		}
		return name
	case LValTemp:
		return fmt.Sprintf("_t%d", v.TempID)
	case LValGlobal:
		return v.Name
	case LValLitInt:
		return fmt.Sprintf("%d", v.IntVal)
	case LValLitUint:
		return fmt.Sprintf("%dU", v.UintVal)
	case LValLitFloat:
		return fmt.Sprintf("%g", v.FloatVal)
	case LValLitString:
		return fmt.Sprintf("FORGE_STR(%q)", v.StrVal)
	case LValLitBool:
		if v.BoolVal {
			return "true"
		}
		return "false"
	case LValLitNull:
		return "NULL"
	case LValIndexRef:
		coll := g.emitValue(v.Collection)
		idx := g.emitValue(v.Index)
		return fmt.Sprintf("%s.data[%s]", coll, idx)
	default:
		return "/* unknown value */"
	}
}

// emitValueAsCStr emits a value as a const char* (for C interop like error messages).
// For string literals, emits the raw C string without FORGE_STR wrapping.
// For forge_string values, extracts .data with a cast.
func (g *cGen) emitValueAsCStr(v *LValue) string {
	if v.Kind == LValLitString {
		return fmt.Sprintf("%q", v.StrVal)
	}
	// If the value is already a const char* (e.g., error type), use it directly
	if v.Kind == LValTemp {
		if ty, ok := g.tempTypes[v.TempID]; ok && ty != nil {
			if ty.Kind == LTyError {
				return g.emitValue(v)
			}
			// Error class handle: extract msg field (Error has msg: string)
			if ty.Kind == LTyClassHandle {
				return fmt.Sprintf("(const char*)%s->msg.data", g.emitValue(v))
			}
		}
		// Also check varTypes — multi-return destructuring creates VarDecls with _tN names
		varName := fmt.Sprintf("_t%d", v.TempID)
		if ty, ok := g.varTypes[varName]; ok && ty != nil {
			if ty.Kind == LTyError {
				return g.emitValue(v)
			}
			if ty.Kind == LTyClassHandle {
				return fmt.Sprintf("(const char*)%s->msg.data", g.emitValue(v))
			}
		}
	}
	if v.Kind == LValVar {
		if ty, ok := g.varTypes[v.Name]; ok && ty != nil {
			if ty.Kind == LTyError {
				return g.emitValue(v)
			}
			if ty.Kind == LTyClassHandle {
				return fmt.Sprintf("(const char*)%s->msg.data", g.emitValue(v))
			}
		}
	}
	return fmt.Sprintf("(const char*)%s.data", g.emitValue(v))
}

// ---------------------------------------------------------------------------
// Builtins
// ---------------------------------------------------------------------------

func (g *cGen) emitBuiltin(d *LBuiltinData) string {
	switch d.Name {
	case "len", "string_len":
		if len(d.Args) > 0 && d.Args[0].Type != nil && d.Args[0].Type.Kind == LTyString {
			return fmt.Sprintf("%s.len", g.emitValue(&d.Args[0]))
		}
		return fmt.Sprintf("%s.len", g.emitValue(&d.Args[0]))

	case "slice_len":
		if len(d.Args) > 0 {
			return fmt.Sprintf("%s.len", g.emitValue(&d.Args[0]))
		}
		return "0"

	case "append", "slice_push":
		if len(d.Args) >= 2 {
			sliceArg := g.emitValue(&d.Args[0])
			elemArg := g.emitValue(&d.Args[1])
			sliceType := g.sliceTypeNameFromValue(&d.Args[0])
			return fmt.Sprintf("({ forge_push(&%s, %s, %s); %s; })",
				sliceArg, elemArg, sliceType, sliceArg)
		}
		return "/* append: missing args */"

	case "push_bytes":
		if len(d.Args) >= 2 {
			dstArg := g.emitValue(&d.Args[0])
			srcArg := g.emitValue(&d.Args[1])
			return fmt.Sprintf("({ forge_push_bytes(&%s, %s); %s; })",
				dstArg, srcArg, dstArg)
		}
		return "/* push_bytes: missing args */"

	case "pop", "slice_pop":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_pop(&%s)", g.emitValue(&d.Args[0]))
		}
		return "/* pop: missing args */"

	case "isnull":
		if len(d.Args) > 0 {
			argType := d.Args[0].Type
			// Resolve through temp/var types for better class handle detection
			if d.Args[0].Kind == LValTemp {
				if resolved, ok := g.tempTypes[d.Args[0].TempID]; ok && resolved != nil {
					argType = resolved
				}
			} else if d.Args[0].Kind == LValVar {
				if resolved, ok := g.varTypes[d.Args[0].Name]; ok && resolved != nil {
					argType = resolved
				}
			}
			if argType != nil && (argType.Kind == LTyClassHandle || argType.Kind == LTyAny ||
				(argType.Kind == LTyOptional && argType.Elem != nil && argType.Elem.Kind == LTyClassHandle)) {
				return fmt.Sprintf("(%s == NULL)", g.emitValue(&d.Args[0]))
			}
			return fmt.Sprintf("forge_isnull(%s)", g.emitValue(&d.Args[0]))
		}
		return "false"

	case "has_prefix", "str_has_prefix", "string_has_prefix":
		if len(d.Args) >= 2 {
			return fmt.Sprintf("forge_str_has_prefix(%s, %s)", g.emitValue(&d.Args[0]), g.emitValue(&d.Args[1]))
		}
	case "has_suffix", "str_has_suffix", "string_has_suffix":
		if len(d.Args) >= 2 {
			return fmt.Sprintf("forge_str_has_suffix(%s, %s)", g.emitValue(&d.Args[0]), g.emitValue(&d.Args[1]))
		}
	case "contains", "str_contains", "string_contains", "slice_contains":
		if len(d.Args) >= 2 {
			if d.Args[0].Type != nil && d.Args[0].Type.Kind == LTyString {
				return fmt.Sprintf("forge_str_contains(%s, %s)", g.emitValue(&d.Args[0]), g.emitValue(&d.Args[1]))
			}
			return fmt.Sprintf("forge_contains(%s, %s)", g.emitValue(&d.Args[0]), g.emitValue(&d.Args[1]))
		}
	case "index_of", "str_index_of", "string_index_of":
		if len(d.Args) >= 2 {
			return fmt.Sprintf("forge_str_index_of(%s, %s)", g.emitValue(&d.Args[0]), g.emitValue(&d.Args[1]))
		}
	case "replace", "str_replace", "string_replace":
		if len(d.Args) >= 3 {
			return fmt.Sprintf("forge_str_replace(%s, %s, %s)", g.emitValue(&d.Args[0]), g.emitValue(&d.Args[1]), g.emitValue(&d.Args[2]))
		}
	case "join", "str_join", "slice_join":
		if len(d.Args) >= 2 {
			slice := g.emitValue(&d.Args[0])
			sep := g.emitValue(&d.Args[1])
			return fmt.Sprintf("forge_str_join(%s, %s.data, %s.len)", sep, slice, slice)
		}
	case "repeat", "str_repeat", "string_repeat":
		if len(d.Args) >= 2 {
			return fmt.Sprintf("forge_str_repeat(%s, %s)", g.emitValue(&d.Args[0]), g.emitValue(&d.Args[1]))
		}
	case "string_to_upper":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_toupper(%s)", g.emitValue(&d.Args[0]))
		}
	case "string_to_lower":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_tolower(%s)", g.emitValue(&d.Args[0]))
		}
	case "string_split":
		sliceType := g.sliceTypeName(&LType{Kind: LTyString})
		return fmt.Sprintf("forge_slice_empty(%s) /* string_split not implemented */", sliceType)
	case "string_trim":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_str_trim(%s)", g.emitValue(&d.Args[0]))
		}
	case "map_len":
		return "0 /* map_len: maps not supported */"
	case "map_contains_key":
		return "false /* map_contains_key: maps not supported */"
	case "contains_key":
		return "false /* contains_key: maps not supported */"
	case "keys", "map_keys":
		if len(d.Args) > 0 && d.Args[0].Type != nil && d.Args[0].Type.Kind == LTyMap && d.Args[0].Type.Key != nil {
			return fmt.Sprintf("forge_slice_empty(%s) /* keys: maps not supported */", g.sliceTypeName(d.Args[0].Type.Key))
		}
		return fmt.Sprintf("forge_slice_empty(%s) /* keys: maps not supported */", g.sliceTypeName(&LType{Kind: LTyString}))
	case "values", "map_values":
		if len(d.Args) > 0 && d.Args[0].Type != nil && d.Args[0].Type.Kind == LTyMap && d.Args[0].Type.Elem != nil {
			return fmt.Sprintf("forge_slice_empty(%s) /* values: maps not supported */", g.sliceTypeName(d.Args[0].Type.Elem))
		}
		return fmt.Sprintf("forge_slice_empty(%s) /* values: maps not supported */", g.sliceTypeName(&LType{Kind: LTyI32}))

	case "channel_receive":
		if len(d.Args) > 0 {
			chanType := d.Args[0].Type
			if chanType == nil {
				if d.Args[0].Kind == LValVar {
					chanType = g.varTypes[d.Args[0].Name]
				} else if d.Args[0].Kind == LValTemp {
					if t, ok := g.tempTypes[d.Args[0].TempID]; ok {
						chanType = t
					}
				}
			}
			suffix := "void"
			if chanType != nil && chanType.Kind == LTyChannel && chanType.Elem != nil {
				suffix = g.chanSuffix(chanType.Elem)
			}
			return fmt.Sprintf("forge_chan_recv_%s(%s)", suffix, g.emitValue(&d.Args[0]))
		}
	case "channel_close":
		if len(d.Args) > 0 {
			chanType := d.Args[0].Type
			if chanType == nil {
				if d.Args[0].Kind == LValVar {
					chanType = g.varTypes[d.Args[0].Name]
				} else if d.Args[0].Kind == LValTemp {
					if t, ok := g.tempTypes[d.Args[0].TempID]; ok {
						chanType = t
					}
				}
			}
			suffix := "void"
			if chanType != nil && chanType.Kind == LTyChannel && chanType.Elem != nil {
				suffix = g.chanSuffix(chanType.Elem)
			}
			return fmt.Sprintf("forge_chan_close_%s(%s)", suffix, g.emitValue(&d.Args[0]))
		}

	case "hash_string":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_hash_string(%s)", g.emitValue(&d.Args[0]))
		}
		return "0"

	case "eprint":
		return g.emitFprint("stderr", d.Args, false)
	case "eprintln":
		return g.emitFprint("stderr", d.Args, true)
	case "read_file":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_read_file(%s)", g.emitValue(&d.Args[0]))
		}
	case "write_file":
		if len(d.Args) >= 2 {
			return fmt.Sprintf("forge_write_file(%s, %s)", g.emitValue(&d.Args[0]), g.emitValue(&d.Args[1]))
		}
	case "os_args":
		g.needsOsArgs = true
		return "_forge_os_args(_argc, _argv)"
	case "os_exit":
		if len(d.Args) > 0 {
			return fmt.Sprintf("exit(%s)", g.emitValue(&d.Args[0]))
		}
		return "exit(0)"
	case "os_getwd":
		return "forge_getwd()"
	case "list_dir":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_list_dir(%s)", g.emitValue(&d.Args[0]))
		}
	case "file_exists":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_file_exists(%s)", g.emitValue(&d.Args[0]))
		}
	case "mkdtemp":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_mkdtemp(%s)", g.emitValue(&d.Args[0]))
		}
	case "exec_command":
		g.needsExecCmd = true
		if len(d.Args) >= 2 {
			return fmt.Sprintf("_forge_exec_command(%s, %s)", g.emitValue(&d.Args[0]), g.emitValue(&d.Args[1]))
		}
	case "path_join":
		g.needsPathJoin = true
		if len(d.Args) > 0 {
			return fmt.Sprintf("_forge_path_join(%s)", g.emitValue(&d.Args[0]))
		}
	case "path_dir":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_path_dir(%s)", g.emitValue(&d.Args[0]))
		}
	case "path_base":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_path_base(%s)", g.emitValue(&d.Args[0]))
		}
	case "path_ext":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_path_ext(%s)", g.emitValue(&d.Args[0]))
		}
	case "itoa":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_itoa(%s)", g.emitValue(&d.Args[0]))
		}
	case "atoi":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_atoi(%s)", g.emitValue(&d.Args[0]))
		}
	case "parse_float":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_parse_float(%s)", g.emitValue(&d.Args[0]))
		}
	case "char_to_string":
		if len(d.Args) > 0 {
			return fmt.Sprintf("forge_char_to_string(%s)", g.emitValue(&d.Args[0]))
		}

	case "println", "Println":
		return g.emitPrintln(d.Args)
	case "print", "Print":
		return g.emitFprint("stdout", d.Args, false)

	case "assert":
		if len(d.Args) >= 2 {
			cond := g.emitValue(&d.Args[0])
			msg := g.emitValue(&d.Args[1])
			return fmt.Sprintf(`forge_assert(%s, %s, %q, %d)`, cond, msg, d.File, d.Line)
		}
	case "assert_eq":
		if len(d.Args) >= 3 {
			actual := &d.Args[0]
			expected := &d.Args[1]
			msg := g.emitValue(&d.Args[2])
			toStringA := g.toStringExpr(actual)
			toStringE := g.toStringExpr(expected)
			eqExpr := g.eqExpr(actual, expected)
			return fmt.Sprintf(`forge_assert_eq(%s, %s, %s, %s, %q, %d)`,
				eqExpr, toStringA, toStringE, msg, d.File, d.Line)
		}
	case "panic":
		if len(d.Args) >= 1 {
			msg := g.emitValue(&d.Args[0])
			return fmt.Sprintf(`forge_panic(%s)`, msg)
		}
	}
	// Generic fallback
	var args []string
	for _, a := range d.Args {
		args = append(args, g.emitValue(&a))
	}
	return fmt.Sprintf("/* builtin %s(%s) */", d.Name, strings.Join(args, ", "))
}

func (g *cGen) emitPrintln(args []LValue) string {
	return g.emitFprint("stdout", args, true)
}

// ---------------------------------------------------------------------------
// Testing support: to_string and equality helpers for assert_eq
// ---------------------------------------------------------------------------

// resolveValueType resolves the LType for a value, checking temp/var type maps.
func (g *cGen) resolveValueType(v *LValue) *LType {
	if v.Type != nil {
		return v.Type
	}
	if v.Kind == LValTemp {
		if t, ok := g.tempTypes[v.TempID]; ok {
			return t
		}
	}
	if v.Kind == LValVar {
		if t, ok := g.varTypes[v.Name]; ok {
			return t
		}
	}
	return nil
}

// toStringExpr returns a C expression that produces a forge_string representation of v.
func (g *cGen) toStringExpr(v *LValue) string {
	t := g.resolveValueType(v)
	val := g.emitValue(v)
	if t == nil {
		return fmt.Sprintf(`FORGE_STR("<unknown>")`)
	}
	switch t.Kind {
	case LTyBool:
		return fmt.Sprintf(`(%s ? FORGE_STR("true") : FORGE_STR("false"))`, val)
	case LTyI8, LTyI16, LTyI32, LTyPlatformInt:
		return fmt.Sprintf(`forge_sprintf("%%d", %s)`, val)
	case LTyI64:
		return fmt.Sprintf(`forge_sprintf("%%lld", (long long)%s)`, val)
	case LTyU8, LTyU16, LTyU32, LTyPlatformUint:
		return fmt.Sprintf(`forge_sprintf("%%u", (unsigned)%s)`, val)
	case LTyU64:
		return fmt.Sprintf(`forge_sprintf("%%llu", (unsigned long long)%s)`, val)
	case LTyF32:
		return fmt.Sprintf(`forge_sprintf("%%g", (double)%s)`, val)
	case LTyF64:
		return fmt.Sprintf(`forge_sprintf("%%g", %s)`, val)
	case LTyString:
		return fmt.Sprintf(`forge_sprintf("\"%%.*s\"", (int)%s.len, (const char*)%s.data)`, val, val)
	case LTyTaggedUnion:
		name := g.structName(t.Name, false)
		return fmt.Sprintf(`%s_to_string(%s)`, name, val)
	case LTyStruct:
		name := g.structName(t.Name, false)
		return fmt.Sprintf(`%s_to_string(%s)`, name, val)
	case LTyClassHandle:
		name := g.structName(t.Name, false)
		return fmt.Sprintf(`(%s == NULL ? FORGE_STR("null") : %s_to_string(%s))`, val, name, val)
	default:
		return fmt.Sprintf(`FORGE_STR("<%s>")`, t.Name)
	}
}

// eqExpr returns a C boolean expression comparing two values for equality.
func (g *cGen) eqExpr(a, b *LValue) string {
	t := g.resolveValueType(a)
	va := g.emitValue(a)
	vb := g.emitValue(b)
	if t != nil && t.Kind == LTyString {
		return fmt.Sprintf("forge_str_eq(%s, %s)", va, vb)
	}
	if t != nil && t.Kind == LTyTaggedUnion {
		name := g.structName(t.Name, false)
		if g.simpleEnums[name] {
			return fmt.Sprintf("(%s == %s)", va, vb)
		}
		return fmt.Sprintf("(%s.tag == %s.tag)", va, vb)
	}
	return fmt.Sprintf("(%s == %s)", va, vb)
}

// emitToStringFunctions emits auto-generated to_string functions for enums, structs, and classes.
func (g *cGen) emitToStringFunctions() {
	// Build set of types that already have a user-defined to_string method
	hasToString := map[string]bool{}
	for _, f := range g.prog.Functions {
		if f.Name == "to_string" && f.Receiver != "" {
			hasToString[f.Receiver] = true
		}
	}

	// Forward-declare ALL to_string functions first.
	// to_string functions may reference each other (e.g., Checker_to_string calls
	// Dict_CType_to_string), so forward decls prevent undeclared function errors.
	for _, e := range g.prog.Enums {
		name := g.structName(e.Name, e.IsExported)
		if hasToString[e.Name] || hasToString[name] {
			continue
		}
		g.linef("static forge_string %s_to_string(%s v);", name, name)
	}
	for _, s := range g.prog.Structs {
		name := g.structName(s.Name, s.IsExported)
		if hasToString[s.Name] || hasToString[name] {
			continue
		}
		g.linef("static forge_string %s_to_string(%s v);", name, name)
	}
	for _, c := range g.prog.Classes {
		name := g.structName(c.Name, c.IsExported)
		if hasToString[c.Name] || hasToString[name] {
			continue
		}
		g.linef("static forge_string %s_to_string(%s* v);", name, name)
	}
	g.line("")

	// Enum to_string: switch on tag, return variant name
	for _, e := range g.prog.Enums {
		name := g.structName(e.Name, e.IsExported)
		if hasToString[e.Name] || hasToString[name] {
			continue
		}
		g.linef("static forge_string %s_to_string(%s v) {", name, name)
		g.indent++
		if g.simpleEnums[name] {
			g.line("switch (v) {")
		} else {
			g.line("switch (v.tag) {")
		}
		g.indent++
		for _, v := range e.Variants {
			g.linef("case %s_%s: return FORGE_STR(\"%s\");", name, v.Name, v.Name)
		}
		g.linef(`default: return FORGE_STR("<unknown %s>");`, e.Name)
		g.indent--
		g.line("}")
		g.indent--
		g.line("}")
		g.line("")
	}

	// Struct to_string: dump all fields
	for _, s := range g.prog.Structs {
		name := g.structName(s.Name, s.IsExported)
		if hasToString[s.Name] || hasToString[name] {
			continue
		}
		g.linef("static forge_string %s_to_string(%s v) {", name, name)
		g.indent++
		g.emitFieldDumpToString(s.Name, s.Fields, "v.")
		g.indent--
		g.line("}")
		g.line("")
	}

	// Class to_string: dump all fields (receiver is pointer)
	for _, c := range g.prog.Classes {
		name := g.structName(c.Name, c.IsExported)
		if hasToString[c.Name] || hasToString[name] {
			continue
		}
		g.linef("static forge_string %s_to_string(%s* v) {", name, name)
		g.indent++
		g.emitFieldDumpToString(c.Name, c.Fields, "v->")
		g.indent--
		g.line("}")
		g.line("")
	}
}

// emitFieldDumpToString emits the body of a to_string function that dumps fields.
// prefix is "v" for structs, "v->" for classes.
func (g *cGen) emitFieldDumpToString(typeName string, fields []LField, prefix string) {
	if len(fields) == 0 {
		g.linef(`return FORGE_STR("%s{}");`, typeName)
		return
	}
	// Build format string and args
	g.linef(`forge_string _result = FORGE_STR("%s{");`, typeName)
	for i, f := range fields {
		fieldVal := prefix + cSafeName(f.Name)
		if i > 0 {
			g.line(`_result = forge_str_concat(_result, FORGE_STR(", "));`)
		}
		g.linef(`_result = forge_str_concat(_result, FORGE_STR("%s: "));`, f.Name)
		fieldStr := g.fieldToStringExpr(f.Type, fieldVal)
		g.linef(`_result = forge_str_concat(_result, %s);`, fieldStr)
	}
	g.line(`_result = forge_str_concat(_result, FORGE_STR("}"));`)
	g.line("return _result;")
}

// fieldToStringExpr returns a C expression that converts a field value to forge_string.
func (g *cGen) fieldToStringExpr(t *LType, val string) string {
	if t == nil {
		return `FORGE_STR("?")`
	}
	switch t.Kind {
	case LTyBool:
		return fmt.Sprintf(`(%s ? FORGE_STR("true") : FORGE_STR("false"))`, val)
	case LTyI8, LTyI16, LTyI32, LTyPlatformInt:
		return fmt.Sprintf(`forge_sprintf("%%d", %s)`, val)
	case LTyI64:
		return fmt.Sprintf(`forge_sprintf("%%lld", (long long)%s)`, val)
	case LTyU8, LTyU16, LTyU32, LTyPlatformUint:
		return fmt.Sprintf(`forge_sprintf("%%u", (unsigned)%s)`, val)
	case LTyU64:
		return fmt.Sprintf(`forge_sprintf("%%llu", (unsigned long long)%s)`, val)
	case LTyF32:
		return fmt.Sprintf(`forge_sprintf("%%g", (double)%s)`, val)
	case LTyF64:
		return fmt.Sprintf(`forge_sprintf("%%g", %s)`, val)
	case LTyString:
		return fmt.Sprintf(`forge_sprintf("\"%%.*s\"", (int)%s.len, (const char*)%s.data)`, val, val)
	case LTyTaggedUnion:
		name := g.structName(t.Name, false)
		return fmt.Sprintf(`%s_to_string(%s)`, name, val)
	case LTyStruct:
		name := g.structName(t.Name, false)
		return fmt.Sprintf(`%s_to_string(%s)`, name, val)
	case LTyClassHandle:
		name := g.structName(t.Name, false)
		return fmt.Sprintf(`(%s == NULL ? FORGE_STR("null") : %s_to_string(%s))`, val, name, val)
	default:
		return fmt.Sprintf(`FORGE_STR("<%s>")`, t.Name)
	}
}

func (g *cGen) emitFprint(stream string, args []LValue, newline bool) string {
	if len(args) == 0 {
		if newline {
			return fmt.Sprintf(`fprintf(%s, "\n")`, stream)
		}
		return fmt.Sprintf(`fprintf(%s, "")`, stream)
	}
	// Check if any arg is a union — requires special handling (can't use printf for unions)
	hasUnion := false
	for _, a := range args {
		t := g.resolveValueType(&a)
		if t != nil && t.Kind == LTyUnion {
			hasUnion = true
			break
		}
	}
	if hasUnion {
		// Emit sequence of fprintf/forge_union_fprint calls
		var stmts []string
		for i, a := range args {
			t := g.resolveValueType(&a)
			if i > 0 {
				stmts = append(stmts, fmt.Sprintf(`fprintf(%s, " ")`, stream))
			}
			if t != nil && t.Kind == LTyUnion {
				stmts = append(stmts, fmt.Sprintf("forge_union_fprint(%s, %s)", stream, g.emitValue(&a)))
			} else {
				spec, argExpr := g.printfSpecAndArg(&a)
				if argExpr != "" {
					stmts = append(stmts, fmt.Sprintf(`fprintf(%s, "%s", %s)`, stream, spec, argExpr))
				} else {
					stmts = append(stmts, fmt.Sprintf(`fprintf(%s, "%s")`, stream, spec))
				}
			}
		}
		if newline {
			stmts = append(stmts, fmt.Sprintf(`fprintf(%s, "\n")`, stream))
		}
		return strings.Join(stmts, "; ")
	}
	var fmtParts []string
	var argParts []string
	for _, a := range args {
		spec, argExpr := g.printfSpecAndArg(&a)
		fmtParts = append(fmtParts, spec)
		if argExpr != "" {
			argParts = append(argParts, argExpr)
		}
	}
	fmtStr := strings.Join(fmtParts, " ")
	if newline {
		fmtStr += `\n`
	}
	if len(argParts) == 0 {
		return fmt.Sprintf(`fprintf(%s, "%s")`, stream, fmtStr)
	}
	return fmt.Sprintf(`fprintf(%s, "%s", %s)`, stream, fmtStr, strings.Join(argParts, ", "))
}

func (g *cGen) emitPrintf(args []LValue) string {
	if len(args) == 0 {
		return `printf("")`
	}
	// Format string is forge_string — extract .data for C's printf
	fmtArg := fmt.Sprintf("(const char*)%s.data", g.emitValue(&args[0]))
	rest := make([]string, 0, len(args)-1)
	for _, a := range args[1:] {
		rest = append(rest, g.emitValue(&a))
	}
	if len(rest) == 0 {
		return fmt.Sprintf("printf(%s)", fmtArg)
	}
	return fmt.Sprintf("printf(%s, %s)", fmtArg, strings.Join(rest, ", "))
}

func (g *cGen) emitSprintf(args []LValue) string {
	if len(args) == 0 {
		return `FORGE_STR_EMPTY`
	}
	// First arg is format string (forge_string — extract .data for C's printf family)
	fmtStr := fmt.Sprintf("(const char*)%s.data", g.emitValue(&args[0]))
	if len(args) == 1 {
		return g.emitValue(&args[0])
	}
	rest := make([]string, 0, len(args)-1)
	for _, a := range args[1:] {
		rest = append(rest, g.emitValue(&a))
	}
	return fmt.Sprintf("forge_sprintf(%s, %s)", fmtStr, strings.Join(rest, ", "))
}

func (g *cGen) emitFormat(d *LFormatData) string {
	var fmtParts []string
	var argParts []string
	for _, p := range d.Parts {
		if p.IsLiteral {
			fmtParts = append(fmtParts, escapeC(p.Text))
		} else {
			spec, argExpr := g.printfSpecAndArg(&p.Value)
			fmtParts = append(fmtParts, spec)
			if argExpr != "" {
				argParts = append(argParts, argExpr)
			}
		}
	}
	fmtStr := strings.Join(fmtParts, "")
	if len(argParts) == 0 {
		return fmt.Sprintf(`FORGE_STR("%s")`, fmtStr)
	}
	return fmt.Sprintf(`forge_sprintf("%s", %s)`,
		fmtStr, strings.Join(argParts, ", "))
}

// printfSpecAndArg returns the format specifier and the argument expression.
// For bools, the argument is wrapped in forge_bool_str().
func (g *cGen) printfSpecAndArg(v *LValue) (string, string) {
	t := v.Type
	// Resolve through temp/var types for better format specifier selection
	if v.Kind == LValTemp {
		if resolved, ok := g.tempTypes[v.TempID]; ok && resolved != nil {
			t = resolved
		} else {
			// Multi-return destructuring creates VarDecls with _tN names
			varName := fmt.Sprintf("_t%d", v.TempID)
			if resolved, ok := g.varTypes[varName]; ok && resolved != nil {
				t = resolved
			}
		}
	} else if v.Kind == LValVar {
		if resolved, ok := g.varTypes[v.Name]; ok && resolved != nil {
			t = resolved
		}
	}
	if t == nil {
		return "%d", g.emitValue(v)
	}
	switch t.Kind {
	case LTyI8, LTyI16, LTyI32, LTyPlatformInt:
		return "%d", g.emitValue(v)
	case LTyI64:
		return "%lld", g.emitValue(v)
	case LTyU8, LTyU16, LTyU32, LTyPlatformUint:
		return "%u", g.emitValue(v)
	case LTyU64:
		return "%llu", g.emitValue(v)
	case LTyF32, LTyF64:
		return "%g", g.emitValue(v)
	case LTyBool:
		return "%s", fmt.Sprintf("forge_bool_str(%s)", g.emitValue(v))
	case LTyString:
		return "%.*s", fmt.Sprintf("(int)%s.len, (const char*)%s.data", g.emitValue(v), g.emitValue(v))
	case LTyError:
		return "%s", g.emitValue(v)
	case LTyTaggedUnion:
		name := g.structName(t.Name, false)
		toStr := fmt.Sprintf("%s_to_string(%s)", name, g.emitValue(v))
		return "%.*s", fmt.Sprintf("(int)%s.len, (const char*)%s.data", toStr, toStr)
	case LTyAny:
		return "%p", g.emitValue(v)
	default:
		return "%d", g.emitValue(v)
	}
}

func (g *cGen) emitArgs(args []LValue, mutArgs []bool) string {
	var parts []string
	for i, a := range args {
		s := g.emitValue(&a)
		if i < len(mutArgs) && mutArgs[i] {
			s = "&" + s
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

// emitArgsBoxed emits function call arguments, boxing concrete types when the
// target function parameter is interface-typed. mutArgs marks which args are passed as `mut` (&x).
func (g *cGen) emitArgsBoxed(funcName string, args []LValue, mutArgs []bool) string {
	// Look up target function to find param types (use original name, not cSafeName)
	fn := g.funcByName[funcName]
	var parts []string
	for i, a := range args {
		argStr := g.emitValue(&a)
		// Check if this arg needs boxing (concrete → interface)
		if fn != nil && i < len(fn.Params) {
			pt := fn.Params[i].Type
			if pt != nil && pt.Kind == LTyUnion {
				// Union param: wrap concrete arg in forge_union_*()
				argType := a.Type
				if argType == nil {
					argType = g.inferLValType(&a)
				}
				if argType != nil && argType.Kind != LTyUnion {
					argStr = g.cWrapUnion(argStr, argType)
				}
			} else if pt != nil && pt.Kind == LTyAny {
				if pt.Name != "" {
					if _, isIface := g.ifaceByName[pt.Name]; isIface {
						// Resolve concrete type of the argument
						concreteClass := g.resolveConcreteClass(&a)
						if concreteClass != "" {
							ifName := g.structName(pt.Name, pt.IsExported)
							className := g.structName(concreteClass, false)
							argStr = fmt.Sprintf("(%s){._data = %s, ._vtable = &%s_as_%s}",
								ifName, argStr, className, ifName)
						}
					}
				} else {
					// Pure any (void*) — box primitives with intptr_t cast
					argType := g.resolveValueType(&a)
					if argType != nil && g.isPrimitiveType(argType) {
						argStr = fmt.Sprintf("(void*)(intptr_t)(%s)", argStr)
					}
				}
			}
		}
		// mut args: pass by pointer
		if i < len(mutArgs) && mutArgs[i] {
			argStr = "&" + argStr
		}
		parts = append(parts, argStr)
	}
	return strings.Join(parts, ", ")
}

// isPrimitiveType returns true for numeric, bool, and string types.
func (g *cGen) isPrimitiveType(t *LType) bool {
	switch t.Kind {
	case LTyI8, LTyI16, LTyI32, LTyI64, LTyU8, LTyU16, LTyU32, LTyU64,
		LTyF32, LTyF64, LTyBool, LTyPlatformInt, LTyPlatformUint:
		return true
	default:
		return false
	}
}

// resolveConcreteClass returns the class name for a value, or "" if unknown.
func (g *cGen) resolveConcreteClass(v *LValue) string {
	t := v.Type
	if t == nil {
		if v.Kind == LValTemp {
			t = g.tempTypes[v.TempID]
		} else if v.Kind == LValVar {
			t = g.varTypes[v.Name]
		}
	}
	if t == nil {
		return ""
	}
	if t.Kind == LTyClassHandle {
		return t.Name
	}
	if t.Kind == LTyOptional && t.Elem != nil && t.Elem.Kind == LTyClassHandle {
		return t.Elem.Name
	}
	return ""
}

// ---------------------------------------------------------------------------
// Type mapping
// ---------------------------------------------------------------------------

func (g *cGen) cType(t *LType) string {
	if t == nil {
		return "void"
	}
	switch t.Kind {
	case LTyI8:
		return "int8_t"
	case LTyI16:
		return "int16_t"
	case LTyI32:
		return "int32_t"
	case LTyI64:
		return "int64_t"
	case LTyU8:
		return "uint8_t"
	case LTyU16:
		return "uint16_t"
	case LTyU32:
		return "uint32_t"
	case LTyU64:
		return "uint64_t"
	case LTyF32:
		return "float"
	case LTyF64:
		return "double"
	case LTyBool:
		return "bool"
	case LTyString:
		return "forge_string"
	case LTyPlatformInt:
		return "int"
	case LTyPlatformUint:
		return "unsigned int"
	case LTySlice:
		return g.sliceTypeName(t.Elem)
	case LTyMap:
		return "void* /* map */"
	case LTyStruct:
		return g.structName(t.Name, t.IsExported)
	case LTyClassHandle:
		name := t.Name
		if g.prog.ClassRenames != nil {
			if renamed, ok := g.prog.ClassRenames[name]; ok {
				name = renamed
			}
		}
		return g.structName(name, t.IsExported) + "*"
	case LTyOptional:
		// Class handles are already pointers in C — NULL = none
		if t.Elem != nil && t.Elem.Kind == LTyClassHandle {
			return g.cType(t.Elem) // ClassName* is already nullable
		}
		return g.optTypeName(t.Elem)
	case LTyTaggedUnion:
		return g.structName(t.Name, t.IsExported)
	case LTyGenerator:
		// Generator type is a pointer to the generator state struct.
		// The struct name is derived from the function that produces it.
		return "void* /* generator */"
	case LTyChannel:
		if t.Elem != nil {
			suffix := g.chanSuffix(t.Elem)
			return fmt.Sprintf("ForgeChan_%s*", suffix)
		}
		return "void* /* channel */"
	case LTyFuncPtr:
		ret := g.cReturnType(t.Return)
		var params []string
		for _, p := range t.Params {
			params = append(params, g.cType(p))
		}
		if len(params) == 0 {
			return fmt.Sprintf("%s (*)(void)", ret)
		}
		return fmt.Sprintf("%s (*)(%s)", ret, strings.Join(params, ", "))
	case LTyUnit:
		return "void"
	case LTyError:
		return "const char*"
	case LTyErrorResult:
		return g.resultTypeName(t.Elem)
	case LTyTuple:
		return g.cTupleType(t)
	case LTyMutex:
		return "pthread_mutex_t"
	case LTyTypeVar:
		return fmt.Sprintf("void* /* typevar %s */", t.Name)
	case LTyAny:
		// If this is a named interface type, use the boxed struct type
		if t.Name != "" {
			if _, isIface := g.ifaceByName[t.Name]; isIface {
				return g.structName(t.Name, t.IsExported)
			}
		}
		return "void*"
	case LTyUnion:
		return "ForgeUnion"
	default:
		return fmt.Sprintf("/* unknown type %d */", t.Kind)
	}
}

func (g *cGen) cFieldDecl(t *LType, name string) string {
	if t != nil && t.Kind == LTyFuncPtr {
		ret := g.cReturnType(t.Return)
		var params []string
		for _, p := range t.Params {
			params = append(params, g.cType(p))
		}
		if len(params) == 0 {
			return fmt.Sprintf("%s (*%s)(void)", ret, name)
		}
		return fmt.Sprintf("%s (*%s)(%s)", ret, name, strings.Join(params, ", "))
	}
	return g.cType(t) + " " + name
}

// ---------------------------------------------------------------------------
// Composite type name helpers
// ---------------------------------------------------------------------------

func (g *cGen) structName(name string, exported bool) string {
	return name
}

func (g *cGen) sliceTypeName(elem *LType) string {
	key := g.cType(elem)
	if name, ok := g.sliceTypes[key]; ok {
		return name
	}
	name := fmt.Sprintf("ForgeSlice_%s", sanitizeCTypeName(key))
	g.sliceTypes[key] = name
	return name
}

func (g *cGen) sliceTypeNameFromType(t *LType) string {
	if t != nil && t.Kind == LTySlice {
		return g.sliceTypeName(t.Elem)
	}
	return "/* unknown slice type */"
}

func (g *cGen) optTypeName(elem *LType) string {
	if elem == nil {
		return "ForgeOpt_void"
	}
	key := g.cType(elem)
	if name, ok := g.optTypes[key]; ok {
		return name
	}
	name := fmt.Sprintf("ForgeOpt_%s", sanitizeCTypeName(key))
	g.optTypes[key] = name
	return name
}

func (g *cGen) optTypeNameFromExpr(e *LExpr) string {
	if e.Type != nil && e.Type.Kind == LTyOptional {
		return g.optTypeName(e.Type.Elem)
	}
	// Fallback: try to get from the wrapped value
	if e.Kind == LExprWrapOptional {
		d := e.Data.(*LWrapOptionalData)
		if d.Value.Type != nil {
			return g.optTypeName(d.Value.Type)
		}
	}
	return "ForgeOpt_void"
}

func (g *cGen) resultTypeName(elem *LType) string {
	if elem == nil {
		return "ForgeResult_void"
	}
	key := g.cType(elem)
	if name, ok := g.resultTypes[key]; ok {
		return name
	}
	name := fmt.Sprintf("ForgeResult_%s", sanitizeCTypeName(key))
	g.resultTypes[key] = name
	return name
}

func (g *cGen) resultTypeNameFromExpr(e *LExpr) string {
	if e.Type != nil && e.Type.Kind == LTyErrorResult && e.Type.Elem != nil {
		return g.resultTypeName(e.Type.Elem)
	}
	return "ForgeResult_void"
}

// Channel type helpers — suffix is used for FORGE_CHAN_DEF/IMPL macros.
func (g *cGen) chanSuffix(elem *LType) string {
	key := g.cType(elem)
	if suffix, ok := g.chanTypes[key]; ok {
		return suffix
	}
	suffix := sanitizeCTypeName(key)
	g.chanTypes[key] = suffix
	return suffix
}

func (g *cGen) chanSuffixFromType(t *LType) string {
	if t != nil && t.Kind == LTyChannel && t.Elem != nil {
		return g.chanSuffix(t.Elem)
	}
	return "void"
}

// collectCompositeTypes walks all types to pre-register slices, optionals, and results.
func (g *cGen) collectCompositeTypes() {
	var containsTypeVar func(t *LType) bool
	containsTypeVar = func(t *LType) bool {
		if t == nil {
			return false
		}
		if t.Kind == LTyTypeVar {
			return true
		}
		return containsTypeVar(t.Elem) || containsTypeVar(t.Key) || containsTypeVar(t.Return)
	}

	var walkType func(t *LType)
	walkType = func(t *LType) {
		if t == nil || containsTypeVar(t) {
			return
		}
		switch t.Kind {
		case LTySlice:
			g.sliceTypeName(t.Elem)
		case LTyOptional:
			// Class handle optionals are just pointers — no ForgeOpt needed
			if t.Elem == nil || t.Elem.Kind != LTyClassHandle {
				g.optTypeName(t.Elem)
			}
		case LTyErrorResult:
			g.resultTypeName(t.Elem)
		case LTyChannel:
			if t.Elem != nil {
				g.chanSuffix(t.Elem)
			}
		case LTyTuple:
			g.cTupleType(t)
		}
		walkType(t.Elem)
		walkType(t.Key)
		walkType(t.Return)
		for _, p := range t.Params {
			walkType(p)
		}
		for _, f := range t.Fields {
			walkType(f.Type)
		}
	}

	walkFuncTypes := func(f *LFuncDecl) {
		walkType(f.ReturnType)
		for _, p := range f.Params {
			walkType(p.Type)
		}
		// Also walk the body for any types used in expressions
		var walkStmts func(stmts []LStmt)
		var walkExpr func(e *LExpr)
		var walkVal func(v *LValue)

		walkVal = func(v *LValue) {
			if v != nil {
				walkType(v.Type)
			}
		}

		walkExpr = func(e *LExpr) {
			if e == nil {
				return
			}
			walkType(e.Type)
			switch e.Kind {
			case LExprCall:
				d := e.Data.(*LCallData)
				for i := range d.Args {
					walkVal(&d.Args[i])
				}
			case LExprBuiltin:
				d := e.Data.(*LBuiltinData)
				for i := range d.Args {
					walkVal(&d.Args[i])
				}
			case LExprMakeSlice:
				// No data — type info is on e.Type
			case LExprMakeChannel:
				d := e.Data.(*LMakeChannelData)
				walkType(d.ElemType)
			case LExprMakeResult:
				d := e.Data.(*LMakeResultData)
				walkVal(&d.Value)
				walkVal(&d.Err)
			case LExprWrapOptional:
				d := e.Data.(*LWrapOptionalData)
				walkVal(&d.Value)
			}
		}

		walkStmts = func(stmts []LStmt) {
			for i := range stmts {
				s := &stmts[i]
				switch s.Kind {
				case LStmtTempDef:
					d := s.Data.(*LTempDef)
					walkExpr(&d.Expr)
				case LStmtVarDecl:
					d := s.Data.(*LVarDecl)
					walkType(d.Type)
					walkVal(d.Init)
				case LStmtSideEffect:
					d := s.Data.(*LSideEffect)
					walkExpr(&d.Expr)
				case LStmtMultiAssign:
					d := s.Data.(*LMultiAssign)
					walkExpr(&d.Expr)
				case LStmtIf:
					d := s.Data.(*LIf)
					walkStmts(d.Then)
					walkStmts(d.Else)
				case LStmtWhile:
					d := s.Data.(*LWhile)
					walkStmts(d.CondBlock)
					walkStmts(d.Body)
				case LStmtFor:
					d := s.Data.(*LFor)
					walkType(d.VarType)
					walkStmts(d.Body)
				case LStmtSwitch:
					d := s.Data.(*LSwitch)
					for _, c := range d.Cases {
						walkStmts(c.Body)
					}
				case LStmtTypeSwitch:
					d := s.Data.(*LTypeSwitch)
					for _, c := range d.Cases {
						walkStmts(c.Body)
					}
				case LStmtBlock:
					d := s.Data.(*LBlock)
					walkStmts(d.Stmts)
				case LStmtSpawn:
					d := s.Data.(*LSpawn)
					walkStmts(d.Body)
				case LStmtSend:
					d := s.Data.(*LSend)
					walkVal(&d.Channel)
					walkVal(&d.Value)
				case LStmtReturn:
					d := s.Data.(*LReturn)
					for i := range d.Values {
						walkVal(&d.Values[i])
					}
				}
			}
		}
		walkStmts(f.Body)
	}

	for i := range g.prog.Functions {
		walkFuncTypes(&g.prog.Functions[i])
	}
	for _, s := range g.prog.Structs {
		for _, f := range s.Fields {
			walkType(f.Type)
		}
	}
	for _, c := range g.prog.Classes {
		for _, f := range c.Fields {
			walkType(f.Type)
		}
	}
	for _, e := range g.prog.Enums {
		for _, v := range e.Variants {
			for _, f := range v.Fields {
				walkType(f.Type)
			}
		}
	}
}

func sanitizeCTypeName(s string) string {
	r := strings.NewReplacer(" ", "_", "*", "ptr", "[", "", "]", "", "(", "", ")", "", ",", "_", "/", "_")
	return r.Replace(s)
}

// isClassOptional returns true if the type is Optional(ClassHandle).
// In C, class handles are already pointers (nullable), so optionals don't need ForgeOpt wrapping.
func (g *cGen) isClassOptional(t *LType) bool {
	return t != nil && t.Kind == LTyOptional && t.Elem != nil && t.Elem.Kind == LTyClassHandle
}

func (g *cGen) visName(name string, exported bool) string {
	return name
}

func (g *cGen) zeroValue(t *LType) string {
	if t == nil {
		return "0"
	}
	switch t.Kind {
	case LTyI8, LTyI16, LTyI32, LTyI64, LTyPlatformInt:
		return "0"
	case LTyU8, LTyU16, LTyU32, LTyU64, LTyPlatformUint:
		return "0"
	case LTyF32, LTyF64:
		return "0.0"
	case LTyBool:
		return "false"
	case LTyString:
		return "FORGE_STR_EMPTY"
	case LTyClassHandle:
		return "NULL"
	case LTyOptional:
		if t.Elem != nil && t.Elem.Kind == LTyClassHandle {
			return "NULL"
		}
		return fmt.Sprintf("forge_none(%s)", g.optTypeName(t.Elem))
	case LTySlice:
		return fmt.Sprintf("forge_slice_empty(%s)", g.sliceTypeName(t.Elem))
	case LTyStruct, LTyTaggedUnion:
		return "{0}"
	default:
		return "{0}"
	}
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func (g *cGen) line(s string) {
	g.buf.WriteString(strings.Repeat("    ", g.indent))
	g.buf.WriteString(s)
	g.buf.WriteByte('\n')
}

func (g *cGen) linef(format string, args ...interface{}) {
	g.line(fmt.Sprintf(format, args...))
}

// resolveFieldType looks up a field type from program declarations.
func (g *cGen) resolveFieldType(ownerType *LType, field string) *LType {
	name := ownerType.Name
	// Apply class renames for monomorphized classes (e.g., DictEntry → DictEntry_i32)
	if g.prog.ClassRenames != nil {
		if renamed, ok := g.prog.ClassRenames[name]; ok {
			name = renamed
		}
	}
	for _, s := range g.prog.Structs {
		if s.Name == name {
			for _, f := range s.Fields {
				if f.Name == field {
					return f.Type
				}
			}
		}
	}
	for _, c := range g.prog.Classes {
		if c.Name == name {
			for _, f := range c.Fields {
				if f.Name == field {
					return f.Type
				}
			}
		}
	}
	return nil
}

func (g *cGen) exprType(e *LExpr) *LType {
	if e == nil || e.Type == nil {
		return &LType{Kind: LTyI32}
	}
	return e.Type
}

func (g *cGen) containsTypeVar(t *LType) bool {
	if t == nil {
		return false
	}
	if t.Kind == LTyTypeVar {
		return true
	}
	return g.containsTypeVar(t.Elem) || g.containsTypeVar(t.Key) || g.containsTypeVar(t.Return)
}

// inferExprType returns the best C type for an expression, resolving LTyAny
// for well-known builtins where the lowerer lost type information.
func (g *cGen) inferExprType(e *LExpr) *LType {
	t := g.exprType(e)
	if t.Kind != LTyAny && !g.containsTypeVar(t) {
		return t
	}
	// Try to infer from function call return type
	if e.Kind == LExprCall {
		d := e.Data.(*LCallData)
		// Known stdlib functions that return strings
		switch d.Func {
		case "strings.ToUpper", "strings.ToLower", "strconv.Itoa":
			return &LType{Kind: LTyString}
		case "fmt.Errorf", "errors.New":
			return &LType{Kind: LTyError}
		}
		for _, fn := range g.prog.Functions {
			if fn.Name == d.Func || (fn.IsExported && fn.Name == cSafeName(d.Func)) {
				if fn.ReturnType != nil && fn.ReturnType.Kind != LTyAny {
					return fn.ReturnType
				}
				break
			}
		}
	}
	// Try to infer from method call return type
	if e.Kind == LExprMethodCall {
		d := e.Data.(*LMethodCallData)
		recvType := d.Receiver.Type
		if recvType == nil {
			if d.Receiver.Kind == LValTemp {
				recvType = g.tempTypes[d.Receiver.TempID]
			} else if d.Receiver.Kind == LValVar {
				recvType = g.varTypes[d.Receiver.Name]
			}
		}
		if recvType != nil {
			// Check for interface method return type
			if recvType.Kind == LTyAny && recvType.Name != "" {
				if iface, ok := g.ifaceByName[recvType.Name]; ok {
					for _, m := range iface.Methods {
						if m.Name == d.Method && m.ReturnType != nil {
							return m.ReturnType
						}
					}
				}
			}
			typeName := recvType.Name
			// Apply class renames for monomorphized classes
			if g.prog.ClassRenames != nil {
				if renamed, ok := g.prog.ClassRenames[typeName]; ok {
					typeName = renamed
				}
			}
			if typeName != "" {
				for _, fn := range g.prog.Functions {
					if fn.Receiver == typeName && fn.Name == d.Method {
						if fn.ReturnType != nil && fn.ReturnType.Kind != LTyAny {
							return fn.ReturnType
						}
						break
					}
				}
			}
		}
	}
	// Try to infer from unwrap (returns optional's inner type)
	if e.Kind == LExprUnwrapOptional {
		d := e.Data.(*LUnwrapOptionalData)
		argType := d.Value.Type
		// Check resolved var types first
		if d.Value.Kind == LValVar {
			if resolved, ok := g.varTypes[d.Value.Name]; ok {
				argType = resolved
			}
		} else if d.Value.Kind == LValTemp {
			if resolved, ok := g.tempTypes[d.Value.TempID]; ok {
				argType = resolved
			}
		}
		if argType != nil && argType.Kind == LTyOptional && argType.Elem != nil {
			return argType.Elem
		}
		// If it's already a class handle, unwrap is identity (class pointers are nullable)
		if argType != nil && argType.Kind == LTyClassHandle {
			return argType
		}
	}
	if e.Kind == LExprBuiltin {
		d := e.Data.(*LBuiltinData)
		if d.Name == "unwrap" && len(d.Args) > 0 {
			argType := d.Args[0].Type
			// Resolve through temp/var types
			if d.Args[0].Kind == LValTemp {
				if resolved, ok := g.tempTypes[d.Args[0].TempID]; ok && resolved != nil {
					argType = resolved
				}
			} else if d.Args[0].Kind == LValVar {
				if resolved, ok := g.varTypes[d.Args[0].Name]; ok && resolved != nil {
					argType = resolved
				}
			}
			if argType != nil && argType.Kind == LTyOptional && argType.Elem != nil {
				return argType.Elem
			}
			// If it's already a class handle, unwrap is identity
			if argType != nil && argType.Kind == LTyClassHandle {
				return argType
			}
		}
	}
	// Try to infer from class/struct field access
	if e.Kind == LExprClassGet {
		d := e.Data.(*LClassGetData)
		if d.Handle.Type != nil {
			fieldType := g.resolveFieldType(d.Handle.Type, d.Field)
			if fieldType != nil {
				return fieldType
			}
		}
	}
	if e.Kind == LExprStructField {
		d := e.Data.(*LStructFieldData)
		if d.Receiver.Type != nil {
			fieldType := g.resolveFieldType(d.Receiver.Type, d.Field)
			if fieldType != nil {
				return fieldType
			}
		}
	}
	// Try to infer from builtin name
	if e.Kind == LExprBuiltin {		d := e.Data.(*LBuiltinData)
		switch d.Name {
		case "len", "string_len", "slice_len", "map_len":
			return &LType{Kind: LTyPlatformInt}
		case "isnull", "contains", "string_contains", "slice_contains",
			"has_prefix", "string_has_prefix", "has_suffix", "string_has_suffix",
			"contains_key", "map_contains_key":
			return &LType{Kind: LTyBool}
		case "index_of", "string_index_of":
			return &LType{Kind: LTyI32}
		case "replace", "string_replace", "repeat", "string_repeat",
			"join", "str_join", "slice_join", "string_trim",
			"string_to_upper", "string_to_lower":
			return &LType{Kind: LTyString}
		case "pop", "slice_pop":
			if len(d.Args) > 0 && d.Args[0].Type != nil && d.Args[0].Type.Kind == LTySlice {
				return d.Args[0].Type.Elem
			}
		case "push", "append", "slice_push", "slice_append", "push_bytes":
			if len(d.Args) > 0 && d.Args[0].Type != nil && d.Args[0].Type.Kind == LTySlice {
				return d.Args[0].Type
			}
		case "keys", "map_keys":
			if len(d.Args) > 0 && d.Args[0].Type != nil && d.Args[0].Type.Kind == LTyMap {
				return &LType{Kind: LTySlice, Elem: d.Args[0].Type.Key}
			}
		case "values", "map_values":
			if len(d.Args) > 0 && d.Args[0].Type != nil && d.Args[0].Type.Kind == LTyMap {
				return &LType{Kind: LTySlice, Elem: d.Args[0].Type.Elem}
			}
		case "itoa", "char_to_string":
			return &LType{Kind: LTyString}
		case "hash_string":
			return &LType{Kind: LTyU64, Bits: 64}
		case "atoi":
			return &LType{Kind: LTyTuple, Fields: []LField{
				{Name: "val", Type: &LType{Kind: LTyI64, Bits: 64}},
				{Name: "ok", Type: &LType{Kind: LTyBool}},
			}}
		case "parse_float":
			return &LType{Kind: LTyTuple, Fields: []LField{
				{Name: "val", Type: &LType{Kind: LTyF64, Bits: 64}},
				{Name: "ok", Type: &LType{Kind: LTyBool}},
			}}
		case "string_split":
			return &LType{Kind: LTySlice, Elem: &LType{Kind: LTyString}}
		case "list_dir", "os_args":
			return &LType{Kind: LTySlice, Elem: &LType{Kind: LTyString}}
		case "file_exists":
			return &LType{Kind: LTyBool}
		case "mkdtemp", "os_getwd":
			return &LType{Kind: LTyString}
		}
	}
	return t
}

func (g *cGen) nextTemp() int {
	id := len(g.tempUsed) + 9000
	g.tempUsed[id] = true
	return id
}

var cReservedWords = map[string]bool{
	"auto": true, "break": true, "case": true, "char": true, "const": true,
	"continue": true, "default": true, "do": true, "double": true, "else": true,
	"enum": true, "extern": true, "float": true, "for": true, "goto": true,
	"if": true, "inline": true, "int": true, "long": true, "register": true,
	"restrict": true, "return": true, "short": true, "signed": true, "sizeof": true,
	"static": true, "struct": true, "switch": true, "typedef": true, "union": true,
	"unsigned": true, "void": true, "volatile": true, "while": true,
}

func cSafeName(name string) string {
	if cReservedWords[name] {
		return name + "_"
	}
	return name
}

// lcFirst lowercases the first character of a string (for C struct field names)

// findClassField looks up a field type by name in a class's field list.
func (g *cGen) findClassField(fields []LField, name string) *LType {
	lcName := lcFirst(name)
	for _, f := range fields {
		if f.Name == name || f.Name == lcName {
			return f.Type
		}
	}
	return nil
}
func lcFirst(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'A' && s[0] <= 'Z' {
		return string(s[0]+32) + s[1:]
	}
	return s
}

func escapeC(s string) string {	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

func cBinOp(op LBinOpKind) string {
	switch op {
	case LBinAdd:
		return "+"
	case LBinSub:
		return "-"
	case LBinMul:
		return "*"
	case LBinDiv:
		return "/"
	case LBinMod:
		return "%"
	case LBinEq:
		return "=="
	case LBinNe:
		return "!="
	case LBinLt:
		return "<"
	case LBinLe:
		return "<="
	case LBinGt:
		return ">"
	case LBinGe:
		return ">="
	case LBinAnd:
		return "&&"
	case LBinOr:
		return "||"
	case LBinBitAnd:
		return "&"
	case LBinBitOr:
		return "|"
	case LBinBitXor:
		return "^"
	case LBinShl:
		return "<<"
	case LBinShr:
		return ">>"
	default:
		return "?"
	}
}

func cUnOp(op LUnOpKind) string {
	switch op {
	case LUnNeg:
		return "-"
	case LUnNot:
		return "!"
	case LUnBitNot:
		return "~"
	default:
		return "?"
	}
}

// collectUsedVars scans LIR statements for all variable names referenced (read or written).
func collectUsedVars(stmts []LStmt) map[string]bool {
	used := map[string]bool{}
	var walkVal func(v *LValue)
	walkVal = func(v *LValue) {
		if v != nil && v.Kind == LValVar {
			used[v.Name] = true
		}
	}
	var walkExpr func(e *LExpr)
	walkExpr = func(e *LExpr) {
		if e == nil {
			return
		}
		switch e.Kind {
		case LExprCall:
			d := e.Data.(*LCallData)
			for i := range d.Args {
				walkVal(&d.Args[i])
			}
		case LExprMethodCall:
			d := e.Data.(*LMethodCallData)
			walkVal(&d.Receiver)
			for i := range d.Args {
				walkVal(&d.Args[i])
			}
		case LExprBuiltin:
			d := e.Data.(*LBuiltinData)
			for i := range d.Args {
				walkVal(&d.Args[i])
			}
		case LExprBinOp:
			d := e.Data.(*LBinOpData)
			walkVal(&d.Left)
			walkVal(&d.Right)
		case LExprUnOp:
			d := e.Data.(*LUnOpData)
			walkVal(&d.Operand)
		case LExprCast:
			d := e.Data.(*LCastData)
			walkVal(&d.Operand)
		case LExprStructField:
			d := e.Data.(*LStructFieldData)
			walkVal(&d.Receiver)
		case LExprClassGet:
			d := e.Data.(*LClassGetData)
			walkVal(&d.Handle)
		case LExprIndexGet:
			d := e.Data.(*LIndexGetData)
			walkVal(&d.Collection)
			walkVal(&d.Index)
		case LExprWrapOptional:
			d := e.Data.(*LWrapOptionalData)
			walkVal(&d.Value)
		case LExprMakeChannel:
			d := e.Data.(*LMakeChannelData)
			if d.BufSize != nil {
				walkVal(d.BufSize)
			}
		}
	}
	var walkStmts func(ss []LStmt)
	walkStmts = func(ss []LStmt) {
		for i := range ss {
			s := &ss[i]
			switch s.Kind {
			case LStmtTempDef:
				d := s.Data.(*LTempDef)
				walkExpr(&d.Expr)
			case LStmtVarDecl:
				d := s.Data.(*LVarDecl)
				if d.Init != nil {
					walkVal(d.Init)
				}
			case LStmtAssign:
				d := s.Data.(*LAssign)
				used[d.Target] = true
				walkVal(&d.Value)
			case LStmtSideEffect:
				d := s.Data.(*LSideEffect)
				walkExpr(&d.Expr)
			case LStmtSend:
				d := s.Data.(*LSend)
				walkVal(&d.Channel)
				walkVal(&d.Value)
			case LStmtIf:
				d := s.Data.(*LIf)
				walkStmts(d.Then)
				walkStmts(d.Else)
			case LStmtWhile:
				d := s.Data.(*LWhile)
				walkStmts(d.CondBlock)
				walkStmts(d.Body)
			case LStmtFor:
				d := s.Data.(*LFor)
				walkStmts(d.Body)
			case LStmtBlock:
				d := s.Data.(*LBlock)
				walkStmts(d.Stmts)
			case LStmtLock:
				d := s.Data.(*LLock)
				walkVal(&d.Mutex)
				walkStmts(d.Body)
			case LStmtReturn:
				d := s.Data.(*LReturn)
				for i := range d.Values {
					walkVal(&d.Values[i])
				}
			}
		}
	}
	walkStmts(stmts)
	return used
}

// collectDeclaredVars scans LIR statements for variable names declared within them.
func collectDeclaredVars(stmts []LStmt) map[string]bool {
	decl := map[string]bool{}
	var walkStmts func(ss []LStmt)
	walkStmts = func(ss []LStmt) {
		for i := range ss {
			s := &ss[i]
			switch s.Kind {
			case LStmtVarDecl:
				d := s.Data.(*LVarDecl)
				decl[d.Name] = true
			case LStmtIf:
				d := s.Data.(*LIf)
				walkStmts(d.Then)
				walkStmts(d.Else)
			case LStmtWhile:
				d := s.Data.(*LWhile)
				walkStmts(d.Body)
			case LStmtFor:
				d := s.Data.(*LFor)
				decl[d.Var] = true
				walkStmts(d.Body)
			case LStmtBlock:
				d := s.Data.(*LBlock)
				walkStmts(d.Stmts)
			case LStmtLock:
				d := s.Data.(*LLock)
				walkStmts(d.Body)
			}
		}
	}
	walkStmts(stmts)
	return decl
}

// EmitTestRunner generates a C main() function that discovers and runs all test_* functions.
// It replaces the normal main() and uses setjmp/longjmp for test isolation.
// testFuncs is a list of test function names found in the program.
func EmitTestRunner(testFuncs []string) string {
	var b strings.Builder
	b.WriteString("\n// --- Test runner (generated by forge test) ---\n")
	b.WriteString("int main(int _argc, char** _argv) {\n")
	b.WriteString("    int _passed = 0, _failed_count = 0, _total = 0;\n")
	b.WriteString("    struct { const char* name; void (*fn)(void); } _tests[] = {\n")
	for _, name := range testFuncs {
		b.WriteString(fmt.Sprintf("        {\"%s\", %s},\n", name, name))
	}
	b.WriteString("    };\n")
	b.WriteString(fmt.Sprintf("    _total = %d;\n", len(testFuncs)))
	b.WriteString("    for (int _i = 0; _i < _total; _i++) {\n")
	b.WriteString("        _forge_test_failed = 0;\n")
	b.WriteString("        if (setjmp(_forge_test_jmp) == 0) {\n")
	b.WriteString("            _tests[_i].fn();\n")
	b.WriteString("        }\n")
	b.WriteString("        if (_forge_test_failed) {\n")
	b.WriteString(`            fprintf(stderr, "FAIL  %s\n", _tests[_i].name);` + "\n")
	b.WriteString("            _failed_count++;\n")
	b.WriteString("        } else {\n")
	b.WriteString(`            fprintf(stderr, "PASS  %s\n", _tests[_i].name);` + "\n")
	b.WriteString("            _passed++;\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n")
	b.WriteString(`    fprintf(stderr, "\n%d tests, %d passed, %d failed\n", _total, _passed, _failed_count);` + "\n")
	b.WriteString("    return _failed_count > 0 ? 1 : 0;\n")
	b.WriteString("}\n")
	return b.String()
}

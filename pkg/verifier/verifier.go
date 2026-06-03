// Package verifier compares .grok understanding files against Go source code,
// reporting structural drift between the declared understanding and the implementation.
package verifier

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	grokast "github.com/waywardgeek/grok/pkg/ast"
	"github.com/waywardgeek/grok/pkg/parser"
)

// Severity classifies how serious a drift finding is.
type Severity int

const (
	Error   Severity = iota // missing type, wrong field type
	Warning                 // extra fields not in .grok, naming convention mismatch
	Info                    // informational (e.g., Go has more methods than .grok declares)
)

func (s Severity) String() string {
	switch s {
	case Error:
		return "ERROR"
	case Warning:
		return "WARNING"
	case Info:
		return "INFO"
	}
	return "UNKNOWN"
}

// Finding is a single drift report.
type Finding struct {
	Severity Severity
	GrokFile string
	GoFile   string
	Message  string
}

func (f Finding) String() string {
	loc := f.GrokFile
	if f.GoFile != "" {
		loc = fmt.Sprintf("%s ↔ %s", f.GrokFile, f.GoFile)
	}
	return fmt.Sprintf("[%s] %s: %s", f.Severity, loc, f.Message)
}

// Result holds all findings from a verification run.
type Result struct {
	Findings []Finding
}

func (r *Result) add(sev Severity, grokFile, goFile, msg string) {
	r.Findings = append(r.Findings, Finding{
		Severity: sev,
		GrokFile: grokFile,
		GoFile:   goFile,
		Message:  msg,
	})
}

// ErrorCount returns the number of error-level findings.
func (r *Result) ErrorCount() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == Error {
			n++
		}
	}
	return n
}

// Verify parses a .grok file and compares it against the Go source files
// referenced in source: annotations. baseDir is the project root used to
// resolve relative source paths.
func Verify(grokPath, baseDir string) (*Result, error) {
	src, err := os.ReadFile(grokPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", grokPath, err)
	}

	grokFile, err := parser.ParseFile(string(src), grokPath)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", grokPath, err)
	}

	result := &Result{}

	for _, block := range grokFile.Blocks {
		if len(block.Source) == 0 {
			result.add(Info, grokPath, "", fmt.Sprintf("grok block %q has no source: annotations", block.Name))
			continue
		}

		// Aggregate all Go types across all source files
		goInfo := &goTypeInfo{
			Structs:    make(map[string]*goStructInfo),
			Interfaces: make(map[string]*goInterfaceInfo),
			Functions:  make(map[string]*goFuncInfo),
			TypeDefs:   make(map[string]bool),
		}

		for _, srcPath := range block.Source {
			goFullPath := filepath.Join(baseDir, srcPath)
			info, err := os.Stat(goFullPath)
			if err != nil {
				if os.IsNotExist(err) {
					result.add(Error, grokPath, srcPath, "source file does not exist")
				} else {
					result.add(Error, grokPath, srcPath, fmt.Sprintf("cannot stat: %v", err))
				}
				continue
			}

			var fileInfo *goTypeInfo
			if info.IsDir() {
				fileInfo, err = parseGoDir(goFullPath)
			} else {
				fileInfo, err = parseGoFile(goFullPath)
			}
			if err != nil {
				result.add(Error, grokPath, srcPath, fmt.Sprintf("failed to parse Go file: %v", err))
				continue
			}
			mergeGoInfo(goInfo, fileInfo)
		}

		// Now compare the grok block against the aggregated Go types
		verifyBlock(block, goInfo, grokPath, result)
	}

	return result, nil
}

// goTypeInfo holds extracted Go type information.
type goTypeInfo struct {
	Structs    map[string]*goStructInfo
	Interfaces map[string]*goInterfaceInfo
	Functions  map[string]*goFuncInfo
	TypeDefs   map[string]bool // type names that are simple typedefs (e.g., type Foo int)
}

type goStructInfo struct {
	Fields  map[string]string // field name → type string
	Methods map[string]*goFuncInfo
}

type goInterfaceInfo struct {
	Methods map[string]*goFuncInfo
}

type goFuncInfo struct {
	Params  []goParam
	Returns []string
}

type goParam struct {
	Name string
	Type string
}

func mergeGoInfo(dst, src *goTypeInfo) {
	for k, v := range src.Structs {
		if existing, ok := dst.Structs[k]; ok {
			// Merge methods
			for mk, mv := range v.Methods {
				existing.Methods[mk] = mv
			}
			for fk, fv := range v.Fields {
				existing.Fields[fk] = fv
			}
		} else {
			dst.Structs[k] = v
		}
	}
	for k, v := range src.Interfaces {
		dst.Interfaces[k] = v
	}
	for k, v := range src.Functions {
		dst.Functions[k] = v
	}
	for k, v := range src.TypeDefs {
		dst.TypeDefs[k] = v
	}
}

func verifyBlock(block grokast.GrokBlock, goInfo *goTypeInfo, grokPath string, result *Result) {
	srcStr := strings.Join(block.Source, ", ")

	for _, s := range block.Structs {
		verifyStruct(s, goInfo, grokPath, srcStr, result)
	}
	for _, c := range block.Classes {
		verifyClass(c, goInfo, grokPath, srcStr, result)
	}
	for _, e := range block.Enums {
		verifyEnum(e, goInfo, grokPath, srcStr, result)
	}
	for _, i := range block.Interfaces {
		verifyInterface(i, goInfo, grokPath, srcStr, result)
	}
	for _, f := range block.Functions {
		verifyFunction(f, goInfo, grokPath, srcStr, result)
	}
}

func parseGoDir(dir string) (*goTypeInfo, error) {
	fset := token.NewFileSet()
	pkgs, err := goparser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		return nil, err
	}

	info := &goTypeInfo{
		Structs:    make(map[string]*goStructInfo),
		Interfaces: make(map[string]*goInterfaceInfo),
		Functions:  make(map[string]*goFuncInfo),
		TypeDefs:   make(map[string]bool),
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			extractGoTypes(file, info)
		}
	}
	return info, nil
}

func parseGoFile(path string) (*goTypeInfo, error) {
	fset := token.NewFileSet()
	file, err := goparser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}

	info := &goTypeInfo{
		Structs:    make(map[string]*goStructInfo),
		Interfaces: make(map[string]*goInterfaceInfo),
		Functions:  make(map[string]*goFuncInfo),
		TypeDefs:   make(map[string]bool),
	}

	extractGoTypes(file, info)
	return info, nil
}

func extractGoTypes(file *ast.File, info *goTypeInfo) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					switch t := ts.Type.(type) {
					case *ast.StructType:
						si := &goStructInfo{
							Fields:  make(map[string]string),
							Methods: make(map[string]*goFuncInfo),
						}
						if t.Fields != nil {
							for _, f := range t.Fields.List {
								typStr := typeExprString(f.Type)
								for _, name := range f.Names {
									si.Fields[name.Name] = typStr
								}
							}
						}
						info.Structs[ts.Name.Name] = si

					case *ast.InterfaceType:
						ii := &goInterfaceInfo{
							Methods: make(map[string]*goFuncInfo),
						}
						if t.Methods != nil {
							for _, m := range t.Methods.List {
								if ft, ok := m.Type.(*ast.FuncType); ok {
									for _, name := range m.Names {
										ii.Methods[name.Name] = extractFuncType(ft)
									}
								}
							}
						}
						info.Interfaces[ts.Name.Name] = ii

					default:
						// Simple typedef (type Foo int, type Bar string, etc.)
						info.TypeDefs[ts.Name.Name] = true
					}
				}
			}

		case *ast.FuncDecl:
			fi := extractFuncInfo(d)
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recvType := receiverTypeName(d.Recv.List[0].Type)
				if recvType != "" {
					si, ok := info.Structs[recvType]
					if !ok {
						si = &goStructInfo{
							Fields:  make(map[string]string),
							Methods: make(map[string]*goFuncInfo),
						}
						info.Structs[recvType] = si
					}
					si.Methods[d.Name.Name] = fi
				}
			} else {
				info.Functions[d.Name.Name] = fi
			}
		}
	}
}

func extractFuncInfo(d *ast.FuncDecl) *goFuncInfo {
	return extractFuncType(d.Type)
}

func extractFuncType(ft *ast.FuncType) *goFuncInfo {
	fi := &goFuncInfo{}
	if ft.Params != nil {
		for _, p := range ft.Params.List {
			typStr := typeExprString(p.Type)
			if len(p.Names) == 0 {
				fi.Params = append(fi.Params, goParam{Type: typStr})
			} else {
				for _, name := range p.Names {
					fi.Params = append(fi.Params, goParam{Name: name.Name, Type: typStr})
				}
			}
		}
	}
	if ft.Results != nil {
		for _, r := range ft.Results.List {
			typStr := typeExprString(r.Type)
			count := len(r.Names)
			if count == 0 {
				count = 1
			}
			for i := 0; i < count; i++ {
				fi.Returns = append(fi.Returns, typStr)
			}
		}
	}
	return fi
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.IndexListExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func typeExprString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeExprString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeExprString(t.Elt)
		}
		return "[...]" + typeExprString(t.Elt)
	case *ast.MapType:
		return "map[" + typeExprString(t.Key) + "]" + typeExprString(t.Value)
	case *ast.SelectorExpr:
		return typeExprString(t.X) + "." + t.Sel.Name
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + typeExprString(t.Value)
	case *ast.Ellipsis:
		return "..." + typeExprString(t.Elt)
	case *ast.IndexExpr:
		return typeExprString(t.X) + "[" + typeExprString(t.Index) + "]"
	case *ast.IndexListExpr:
		var parts []string
		for _, idx := range t.Indices {
			parts = append(parts, typeExprString(idx))
		}
		return typeExprString(t.X) + "[" + strings.Join(parts, ", ") + "]"
	default:
		return "?"
	}
}

// ---- Grok type → Go type string conversion ----

// grokTypeToGoString converts a grok TypeExpr into the string format that
// typeExprString produces from Go AST, enabling structural comparison.
func grokTypeToGoString(t grokast.TypeExpr) string {
	switch t.Kind {
	case grokast.TypeNamed:
		if t.Data == nil {
			return "?"
		}
		nt := t.Data.(grokast.NamedType)
		name := grokNameToGo(nt.Name)
		if len(nt.Args) == 0 {
			return name
		}
		// Generic: Stack<T> → Stack[T], Map<K,V> → Map[K, V]
		var args []string
		for _, a := range nt.Args {
			args = append(args, grokTypeToGoString(a))
		}
		return name + "[" + strings.Join(args, ", ") + "]"

	case grokast.TypeOptional:
		if t.Data == nil {
			return "?"
		}
		ot := t.Data.(grokast.OptionalType)
		// T? in grok → *T in Go
		return "*" + grokTypeToGoString(ot.Inner)

	case grokast.TypeSequence:
		if t.Data == nil {
			return "[]?"
		}
		st := t.Data.(grokast.SequenceType)
		return "[]" + grokTypeToGoString(st.Elem)

	case grokast.TypeMap:
		if t.Data == nil {
			return "map[?]?"
		}
		mt := t.Data.(grokast.MapType)
		return "map[" + grokTypeToGoString(mt.Key) + "]" + grokTypeToGoString(mt.Value)

	case grokast.TypeTuple:
		// Go doesn't have tuples; skip comparison
		return "?"

	case grokast.TypeFunc:
		return "func(...)"

	case grokast.TypeChannel:
		if t.Data == nil {
			return "chan ?"
		}
		ct := t.Data.(grokast.ChannelType)
		return "chan " + grokTypeToGoString(ct.Elem)

	case grokast.TypeLock:
		return "sync.Mutex"

	case grokast.TypeUnit:
		return ""

	default:
		return "?"
	}
}

// grokNameToGo maps grok primitive type names to Go equivalents.
func grokNameToGo(name string) string {
	switch name {
	case "string":
		return "string"
	case "bool":
		return "bool"
	case "i8":
		return "int8"
	case "i16":
		return "int16"
	case "i32":
		return "int32"
	case "i64":
		return "int64"
	case "i128", "i256":
		return name // no Go equivalent, keep as-is
	case "u8":
		return "uint8"
	case "u16":
		return "uint16"
	case "u32":
		return "uint32"
	case "u64":
		return "uint64"
	case "f32":
		return "float32"
	case "f64":
		return "float64"
	case "f128":
		return name
	case "int":
		return "int"
	case "any":
		return "any"
	case "error":
		return "error"
	default:
		return name // user-defined types pass through
	}
}

// typesMatch compares a grok type string against a Go type string.
// Returns true if they match or if the grok type is "?" (unknown/unconvertible).
func typesMatch(grokStr, goStr string) bool {
	if grokStr == "?" || grokStr == "" {
		return true // can't compare, don't report false positive
	}
	if grokStr == goStr {
		return true
	}
	// Strip Go package prefix: "ast.Span" → "Span" for comparison
	// since .grok files use unqualified type names
	if idx := strings.LastIndex(goStr, "."); idx >= 0 {
		if grokStr == goStr[idx+1:] {
			return true
		}
	}
	return false
}

// ---- Verification helpers ----

// findGoName checks if a name exists in a map, trying both PascalCase and camelCase.
// Returns the found name and whether it was found.
func findGoName(name string, names map[string]*goFuncInfo) (string, bool) {
	pascal := snakeToPascal(name)
	if _, ok := names[pascal]; ok {
		return pascal, true
	}
	camel := snakeToCamel(name)
	if _, ok := names[camel]; ok {
		return camel, true
	}
	// Try exact match
	if _, ok := names[name]; ok {
		return name, true
	}
	return pascal, false
}

func verifyStruct(s grokast.StructDecl, goInfo *goTypeInfo, grokFile, goFile string, result *Result) {
	goStruct, ok := goInfo.Structs[s.Name]
	if !ok {
		result.add(Error, grokFile, goFile, fmt.Sprintf("struct %s declared in .grok but not found in Go", s.Name))
		return
	}

	for _, grokField := range s.Fields {
		goType, found := findGoFieldType(grokField.Name, goStruct.Fields)
		if !found {
			result.add(Error, grokFile, goFile, fmt.Sprintf("struct %s: field %s not found in Go", s.Name, grokField.Name))
			continue
		}
		grokType := grokTypeToGoString(grokField.Type)
		if !typesMatch(grokType, goType) {
			result.add(Error, grokFile, goFile, fmt.Sprintf("struct %s: field %s type mismatch: .grok=%s, Go=%s", s.Name, grokField.Name, grokType, goType))
		}
	}

	grokFieldSet := make(map[string]bool)
	for _, f := range s.Fields {
		grokFieldSet[snakeToPascal(f.Name)] = true
		grokFieldSet[snakeToCamel(f.Name)] = true
		grokFieldSet[f.Name] = true
	}
	var extras []string
	for goField := range goStruct.Fields {
		if !grokFieldSet[goField] {
			extras = append(extras, goField)
		}
	}
	sort.Strings(extras)
	for _, extra := range extras {
		result.add(Warning, grokFile, goFile, fmt.Sprintf("struct %s: Go has field %s not in .grok", s.Name, extra))
	}
}

func verifyClass(c grokast.ClassDecl, goInfo *goTypeInfo, grokFile, goFile string, result *Result) {
	goStruct, ok := goInfo.Structs[c.Name]
	if !ok {
		result.add(Error, grokFile, goFile, fmt.Sprintf("class %s declared in .grok but not found as Go struct", c.Name))
		return
	}

	for _, grokField := range c.Fields {
		goType, found := findGoFieldType(grokField.Name, goStruct.Fields)
		if !found {
			result.add(Error, grokFile, goFile, fmt.Sprintf("class %s: field %s not found in Go", c.Name, grokField.Name))
			continue
		}
		grokType := grokTypeToGoString(grokField.Type)
		if !typesMatch(grokType, goType) {
			result.add(Error, grokFile, goFile, fmt.Sprintf("class %s: field %s type mismatch: .grok=%s, Go=%s", c.Name, grokField.Name, grokType, goType))
		}
	}

	for _, param := range c.CtorParams {
		if param.IsSelf {
			continue
		}
		if !findGoField(param.Name, goStruct.Fields) {
			result.add(Warning, grokFile, goFile, fmt.Sprintf("class %s: ctor param %s not found as field", c.Name, param.Name))
		}
	}

	for _, grokMethod := range c.Methods {
		goName, found := findGoName(grokMethod.Name, goStruct.Methods)
		if !found {
			pascal := snakeToPascal(grokMethod.Name)
			camel := snakeToCamel(grokMethod.Name)
			result.add(Error, grokFile, goFile, fmt.Sprintf("class %s: method %s (tried Go: %s, %s) not found", c.Name, grokMethod.Name, pascal, camel))
			continue
		}
		goFunc := goStruct.Methods[goName]
		verifyFuncSignature(fmt.Sprintf("class %s method %s", c.Name, grokMethod.Name), grokMethod, goFunc, grokFile, goFile, result)
	}

	grokMethodSet := make(map[string]bool)
	for _, m := range c.Methods {
		grokMethodSet[snakeToPascal(m.Name)] = true
		grokMethodSet[snakeToCamel(m.Name)] = true
		grokMethodSet[m.Name] = true
	}
	var extras []string
	for goMethod := range goStruct.Methods {
		if !grokMethodSet[goMethod] {
			extras = append(extras, goMethod)
		}
	}
	sort.Strings(extras)
	for _, extra := range extras {
		result.add(Info, grokFile, goFile, fmt.Sprintf("class %s: Go has method %s not in .grok", c.Name, extra))
	}
}

func verifyEnum(e grokast.EnumDecl, goInfo *goTypeInfo, grokFile, goFile string, result *Result) {
	// Go enums are typically: type FooKind int (typedef) or type Foo int
	// Check for the type as a typedef, struct, or interface
	_, hasStruct := goInfo.Structs[e.Name]
	_, hasIface := goInfo.Interfaces[e.Name]
	_, hasTypedef := goInfo.TypeDefs[e.Name]

	if hasStruct || hasIface || hasTypedef {
		return
	}

	// Also check XxxKind pattern
	kindName := e.Name + "Kind"
	if goInfo.TypeDefs[kindName] {
		return
	}

	result.add(Warning, grokFile, goFile, fmt.Sprintf("enum %s: no matching Go type found (looked for %s, %s as typedef/struct/interface)", e.Name, e.Name, kindName))
}

func verifyInterface(i grokast.InterfaceDecl, goInfo *goTypeInfo, grokFile, goFile string, result *Result) {
	goIface, ok := goInfo.Interfaces[i.Name]
	if !ok {
		result.add(Error, grokFile, goFile, fmt.Sprintf("interface %s declared in .grok but not found in Go", i.Name))
		return
	}

	for _, grokMethod := range i.Methods {
		goName := toGoMethodName(grokMethod.Name)
		goFunc, ok := goIface.Methods[goName]
		if !ok {
			result.add(Error, grokFile, goFile, fmt.Sprintf("interface %s: method %s (Go: %s) not found", i.Name, grokMethod.Name, goName))
			continue
		}
		verifyFuncSignature(fmt.Sprintf("interface %s method %s", i.Name, grokMethod.Name), grokMethod, goFunc, grokFile, goFile, result)
	}
}

func verifyFunction(f grokast.FuncDecl, goInfo *goTypeInfo, grokFile, goFile string, result *Result) {
	pascal := snakeToPascal(f.Name)
	camel := snakeToCamel(f.Name)

	var goFunc *goFuncInfo
	if fn, ok := goInfo.Functions[pascal]; ok {
		goFunc = fn
	} else if fn, ok := goInfo.Functions[camel]; ok {
		goFunc = fn
	} else if fn, ok := goInfo.Functions[f.Name]; ok {
		goFunc = fn
	}

	if goFunc == nil {
		result.add(Error, grokFile, goFile, fmt.Sprintf("function %s (tried Go: %s, %s) not found", f.Name, pascal, camel))
		return
	}

	verifyFuncSignature(fmt.Sprintf("function %s", f.Name), f, goFunc, grokFile, goFile, result)
}

// ---- Naming convention helpers ----

// toGoFieldName tries PascalCase first (exported), then the original name (unexported).
func toGoFieldName(name string) string {
	return snakeToPascal(name)
}

// findGoField checks if a field exists, trying PascalCase, camelCase, and exact match.
func findGoField(name string, fields map[string]string) bool {
	_, found := findGoFieldType(name, fields)
	return found
}

// findGoFieldType returns the Go type string for a field, trying PascalCase, camelCase, and exact match.
func findGoFieldType(name string, fields map[string]string) (string, bool) {
	pascal := snakeToPascal(name)
	if typ, ok := fields[pascal]; ok {
		return typ, true
	}
	camel := snakeToCamel(name)
	if typ, ok := fields[camel]; ok {
		return typ, true
	}
	if typ, ok := fields[name]; ok {
		return typ, true
	}
	return "", false
}

// verifyFuncSignature checks parameter count and return type of a grok function against Go.
func verifyFuncSignature(context string, grokFunc grokast.FuncDecl, goFunc *goFuncInfo, grokFile, goFile string, result *Result) {
	// Count grok params excluding self
	grokParamCount := 0
	for _, p := range grokFunc.Params {
		if !p.IsSelf {
			grokParamCount++
		}
	}
	goParamCount := len(goFunc.Params)

	if grokParamCount != goParamCount {
		result.add(Error, grokFile, goFile, fmt.Sprintf("%s: param count mismatch: .grok=%d, Go=%d", context, grokParamCount, goParamCount))
	} else {
		// Check param types positionally
		gi := 0
		for _, grokParam := range grokFunc.Params {
			if grokParam.IsSelf {
				continue
			}
			if gi < len(goFunc.Params) {
				grokType := grokTypeToGoString(grokParam.Type)
				goType := goFunc.Params[gi].Type
				if !typesMatch(grokType, goType) {
					paramName := grokParam.Name
					if paramName == "" {
						paramName = fmt.Sprintf("#%d", gi+1)
					}
					result.add(Error, grokFile, goFile, fmt.Sprintf("%s: param %s type mismatch: .grok=%s, Go=%s", context, paramName, grokType, goType))
				}
			}
			gi++
		}
	}

	// Check return types
	grokReturnCount := 0
	var grokReturnStr string
	if grokFunc.ReturnType != nil {
		grokReturnStr = grokTypeToGoString(*grokFunc.ReturnType)
		if grokFunc.ReturnType.Kind == grokast.TypeTuple {
			if grokFunc.ReturnType.Data != nil {
				tt := grokFunc.ReturnType.Data.(grokast.TupleType)
				grokReturnCount = len(tt.Fields)
			}
		} else if grokFunc.ReturnType.Kind == grokast.TypeUnit {
			grokReturnCount = 0
		} else {
			grokReturnCount = 1
		}
	}
	goReturnCount := len(goFunc.Returns)

	if grokReturnCount != goReturnCount {
		result.add(Error, grokFile, goFile, fmt.Sprintf("%s: return count mismatch: .grok=%d, Go=%d", context, grokReturnCount, goReturnCount))
	} else if grokReturnCount == 1 && goReturnCount == 1 {
		if !typesMatch(grokReturnStr, goFunc.Returns[0]) {
			result.add(Error, grokFile, goFile, fmt.Sprintf("%s: return type mismatch: .grok=%s, Go=%s", context, grokReturnStr, goFunc.Returns[0]))
		}
	} else if grokReturnCount > 1 && grokFunc.ReturnType != nil && grokFunc.ReturnType.Kind == grokast.TypeTuple {
		// Element-wise comparison for tuple returns
		tt := grokFunc.ReturnType.Data.(grokast.TupleType)
		for i, field := range tt.Fields {
			if i < len(goFunc.Returns) {
				grokElemStr := grokTypeToGoString(field.Type)
				if !typesMatch(grokElemStr, goFunc.Returns[i]) {
					result.add(Error, grokFile, goFile, fmt.Sprintf("%s: return #%d type mismatch: .grok=%s, Go=%s", context, i+1, grokElemStr, goFunc.Returns[i]))
				}
			}
		}
	}
}

func toGoMethodName(name string) string {
	return snakeToPascal(name)
}

// snakeToPascal converts snake_case to PascalCase.
func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	var result strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		result.WriteString(strings.ToUpper(p[:1]))
		result.WriteString(p[1:])
	}
	return result.String()
}

// snakeToCamel converts snake_case to camelCase.
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	var result strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 {
			result.WriteString(p)
		} else {
			result.WriteString(strings.ToUpper(p[:1]))
			result.WriteString(p[1:])
		}
	}
	return result.String()
}

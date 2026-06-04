package main

import (
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// runGen scaffolds a new .grok file from a Go package directory.
func runGen(pkgDir string) error {
	absDir, err := filepath.Abs(pkgDir)
	if err != nil {
		return err
	}

	goFiles, err := scanGoFiles(absDir)
	if err != nil {
		return err
	}
	if len(goFiles) == 0 {
		return fmt.Errorf("no .go files found in %s", pkgDir)
	}

	fset := token.NewFileSet()
	var allFiles []*goast.File
	var fileNames []string
	for _, name := range goFiles {
		f, err := goparser.ParseFile(fset, filepath.Join(absDir, name), nil, goparser.ParseComments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", name, err)
			continue
		}
		allFiles = append(allFiles, f)
		fileNames = append(fileNames, name)
	}
	if len(allFiles) == 0 {
		return fmt.Errorf("no parseable .go files")
	}

	pkgName := allFiles[0].Name.Name

	var structs []structInfo
	var interfaces []ifaceInfo
	var typeAliases []aliasInfo
	var funcs []funcEntry
	importSet := make(map[string]bool)

	for i, f := range allFiles {
		baseName := fileNames[i]
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *goast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*goast.TypeSpec)
					if !ok || !isExported(ts.Name.Name) {
						continue
					}
					switch t := ts.Type.(type) {
					case *goast.StructType:
						structs = append(structs, extractStruct(ts.Name.Name, t, ts.TypeParams))
					case *goast.InterfaceType:
						interfaces = append(interfaces, extractInterface(ts.Name.Name, t, ts.TypeParams))
					default:
						typeAliases = append(typeAliases, aliasInfo{
							Name: ts.Name.Name,
							Type: goTypeToGrok(typeString(ts.Type)),
						})
					}
				}
			case *goast.FuncDecl:
				entry := funcEntry{
					File: baseName,
					Line: fset.Position(d.Pos()).Line,
				}
				if d.Doc != nil && len(d.Doc.List) > 0 {
					entry.DocLine = strings.TrimPrefix(d.Doc.List[0].Text, "// ")
					entry.DocLine = strings.TrimPrefix(entry.DocLine, "/* ")
				}
				entry.Signature = buildSignature(d, fset)
				funcs = append(funcs, entry)
			}
		}
		for _, imp := range f.Imports {
			importSet[strings.Trim(imp.Path.Value, `"`)] = true
		}
	}

	sort.Slice(funcs, func(i, j int) bool {
		if funcs[i].File != funcs[j].File {
			return funcs[i].File < funcs[j].File
		}
		return funcs[i].Line < funcs[j].Line
	})

	// Build output
	var b strings.Builder
	blockName := exportName(pkgName)
	b.WriteString(fmt.Sprintf("// %s.grok — structural understanding of the %s package\n\n", pkgName, pkgName))
	b.WriteString(fmt.Sprintf("grok %s {\n", blockName))
	b.WriteString("  why: \"\"\n\n")

	for _, s := range structs {
		b.WriteString(fmt.Sprintf("  struct %s", s.Name))
		if s.TypeParams != "" {
			b.WriteString(fmt.Sprintf("<%s>", s.TypeParams))
		}
		b.WriteString(" {\n")
		for _, f := range s.Fields {
			b.WriteString(fmt.Sprintf("    %s: %s\n", f.Name, f.Type))
		}
		b.WriteString("  }\n\n")
	}

	for _, iface := range interfaces {
		b.WriteString(fmt.Sprintf("  interface %s", iface.Name))
		if iface.TypeParams != "" {
			b.WriteString(fmt.Sprintf("<%s>", iface.TypeParams))
		}
		b.WriteString(" {\n")
		for _, m := range iface.Methods {
			b.WriteString(fmt.Sprintf("    %s\n", m))
		}
		b.WriteString("  }\n\n")
	}

	for _, a := range typeAliases {
		b.WriteString(fmt.Sprintf("  type %s = %s\n\n", a.Name, a.Type))
	}

	b.WriteString(fmt.Sprintf("  source: [%s]\n", formatSourceList(fileNames)))
	b.WriteString("}\n")
	b.WriteString(buildFunctionIndex(funcs, fileNames))
	modPath := findModulePath(absDir)
	b.WriteString(buildDependencies(importSet, modPath))

	outPath := filepath.Join(absDir, pkgName+".grok")
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("%s already exists — use grok update instead", outPath)
	}

	fmt.Printf("generated %s\n", outPath)
	return os.WriteFile(outPath, []byte(b.String()), 0644)
}

// --- Gen types and helpers ---

type fieldInfo struct {
	Name string
	Type string
}

type structInfo struct {
	Name       string
	TypeParams string
	Fields     []fieldInfo
}

type ifaceInfo struct {
	Name       string
	TypeParams string
	Methods    []string
}

type aliasInfo struct {
	Name string
	Type string
}

func extractStruct(name string, st *goast.StructType, typeParams *goast.FieldList) structInfo {
	s := structInfo{Name: name}
	if typeParams != nil {
		s.TypeParams = grokTypeParams(typeParams)
	}
	if st.Fields != nil {
		for _, f := range st.Fields.List {
			grokType := goTypeToGrok(typeString(f.Type))
			if len(f.Names) == 0 {
				s.Fields = append(s.Fields, fieldInfo{Name: typeString(f.Type), Type: grokType})
			} else {
				for _, n := range f.Names {
					if !isExported(n.Name) {
						continue
					}
					s.Fields = append(s.Fields, fieldInfo{Name: n.Name, Type: grokType})
				}
			}
		}
	}
	return s
}

func extractInterface(name string, it *goast.InterfaceType, typeParams *goast.FieldList) ifaceInfo {
	iface := ifaceInfo{Name: name}
	if typeParams != nil {
		iface.TypeParams = grokTypeParams(typeParams)
	}
	if it.Methods != nil {
		for _, m := range it.Methods.List {
			if len(m.Names) == 0 {
				iface.Methods = append(iface.Methods, typeString(m.Type))
				continue
			}
			ft, ok := m.Type.(*goast.FuncType)
			if !ok {
				continue
			}
			iface.Methods = append(iface.Methods, formatGrokMethod(m.Names[0].Name, ft))
		}
	}
	return iface
}

func formatGrokMethod(name string, ft *goast.FuncType) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("func %s(", name))
	if ft.Params != nil {
		var params []string
		for _, p := range ft.Params.List {
			grokType := goTypeToGrok(typeString(p.Type))
			if len(p.Names) == 0 {
				params = append(params, grokType)
			} else {
				for _, n := range p.Names {
					params = append(params, fmt.Sprintf("%s: %s", n.Name, grokType))
				}
			}
		}
		b.WriteString(strings.Join(params, ", "))
	}
	b.WriteString(")")
	if ft.Results != nil && len(ft.Results.List) > 0 {
		results := ft.Results.List
		if len(results) == 1 && len(results[0].Names) == 0 {
			b.WriteString(" -> ")
			b.WriteString(goTypeToGrok(typeString(results[0].Type)))
		} else {
			b.WriteString(" -> (")
			var rets []string
			for _, r := range results {
				rets = append(rets, goTypeToGrok(typeString(r.Type)))
			}
			b.WriteString(strings.Join(rets, ", "))
			b.WriteString(")")
		}
	}
	return b.String()
}

func grokTypeParams(fl *goast.FieldList) string {
	var parts []string
	for _, f := range fl.List {
		constraint := typeString(f.Type)
		for _, n := range f.Names {
			if constraint != "" && constraint != "any" {
				parts = append(parts, fmt.Sprintf("%s: %s", n.Name, constraint))
			} else {
				parts = append(parts, n.Name)
			}
		}
	}
	return strings.Join(parts, ", ")
}

func goTypeToGrok(goType string) string {
	switch goType {
	case "int8":
		return "i8"
	case "int16":
		return "i16"
	case "int32":
		return "i32"
	case "int64":
		return "i64"
	case "uint8":
		return "u8"
	case "uint16":
		return "u16"
	case "uint32":
		return "u32"
	case "uint64":
		return "u64"
	case "float32":
		return "f32"
	case "float64":
		return "f64"
	case "interface{}":
		return "any"
	case "string", "bool", "int", "error", "any":
		return goType
	}
	if strings.HasPrefix(goType, "*") {
		return goTypeToGrok(goType[1:]) + "?"
	}
	if strings.HasPrefix(goType, "[]") {
		return "[" + goTypeToGrok(goType[2:]) + "]"
	}
	if strings.HasPrefix(goType, "map[") {
		depth := 0
		for i, c := range goType {
			if c == '[' {
				depth++
			} else if c == ']' {
				depth--
				if depth == 0 {
					return fmt.Sprintf("map[%s]%s", goTypeToGrok(goType[4:i]), goTypeToGrok(goType[i+1:]))
				}
			}
		}
	}
	return goType
}

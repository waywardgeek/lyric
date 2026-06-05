package lir

import (
	"fmt"
	"strings"
)

// GoBackend emits Go source code from an LIR program.
// This should be under 1,500 lines — all semantic complexity
// lives in the lowering pass.
type GoBackend struct {
	buf     strings.Builder
	indent  int
	imports map[string]string // path → alias (empty = no alias)

	// For method receivers
	currentReceiver string
}

// EmitGo converts an LIR program to Go source code.
func EmitGo(prog *LProgram) string {
	g := &GoBackend{
		imports: make(map[string]string),
	}
	return g.emit(prog)
}

func (g *GoBackend) emit(prog *LProgram) string {
	// Collect imports
	for _, imp := range prog.Imports {
		g.imports[imp.Path] = imp.Alias
	}

	// First pass: emit everything to a buffer to discover auto-imports
	var body strings.Builder

	// Type definitions
	for _, td := range prog.TypeDefs {
		g.buf.Reset()
		g.emitTypeDef(&td)
		body.WriteString(g.buf.String())
	}

	// Structs
	for _, s := range prog.Structs {
		g.buf.Reset()
		g.emitStructDecl(&s)
		body.WriteString(g.buf.String())
	}

	// Enums (tagged unions)
	for _, e := range prog.Enums {
		g.buf.Reset()
		g.emitEnumDecl(&e)
		body.WriteString(g.buf.String())
	}

	// Classes (Go structs with pointer receivers)
	for _, c := range prog.Classes {
		g.buf.Reset()
		g.emitClassDecl(&c)
		body.WriteString(g.buf.String())
	}

	// Functions
	for _, f := range prog.Functions {
		g.buf.Reset()
		g.emitFuncDecl(&f)
		body.WriteString(g.buf.String())
	}

	// Build final output with package + imports + body
	var out strings.Builder
	out.WriteString(fmt.Sprintf("package %s\n\n", prog.Package))

	if len(g.imports) > 0 {
		out.WriteString("import (\n")
		for path, alias := range g.imports {
			if alias != "" {
				out.WriteString(fmt.Sprintf("\t%s %q\n", alias, path))
			} else {
				out.WriteString(fmt.Sprintf("\t%q\n", path))
			}
		}
		out.WriteString(")\n\n")
	}

	out.WriteString(body.String())
	return out.String()
}

// ---------------------------------------------------------------------------
// Type emission
// ---------------------------------------------------------------------------

func (g *GoBackend) goType(t *LType) string {
	if t == nil {
		return ""
	}
	switch t.Kind {
	case LTyI8:
		return "int8"
	case LTyI16:
		return "int16"
	case LTyI32:
		return "int32"
	case LTyI64:
		return "int64"
	case LTyU8:
		return "uint8"
	case LTyU16:
		return "uint16"
	case LTyU32:
		return "uint32"
	case LTyU64:
		return "uint64"
	case LTyF32:
		return "float32"
	case LTyF64:
		return "float64"
	case LTyBool:
		return "bool"
	case LTyString:
		return "string"
	case LTyUnit:
		return ""
	case LTyError:
		return "error"
	case LTyPlatformInt:
		return "int"
	case LTyPlatformUint:
		return "uint"
	case LTyStruct:
		return t.Name
	case LTyClassHandle:
		return "*" + t.Name
	case LTyTuple:
		// Go doesn't have tuples — this case shouldn't appear in emitted code
		return "any"
	case LTySlice:
		return "[]" + g.goType(t.Elem)
	case LTyMap:
		return "map[" + g.goType(t.Key) + "]" + g.goType(t.Elem)
	case LTyChannel:
		return "chan " + g.goType(t.Elem)
	case LTyMutex:
		g.autoImport("sync")
		return "sync.Mutex"
	case LTyOptional:
		return "*" + g.goType(t.Elem)
	case LTyTaggedUnion:
		return t.Name
	case LTyFuncPtr:
		var params []string
		for _, p := range t.Params {
			params = append(params, g.goType(p))
		}
		ret := ""
		if t.Return != nil && t.Return.Kind != LTyUnit {
			ret = " " + g.goType(t.Return)
		}
		return fmt.Sprintf("func(%s)%s", strings.Join(params, ", "), ret)
	case LTyErrorResult:
		// This is (T, error) — emitted as multi-return, not a single type
		return g.goType(t.Elem)
	case LTyAny:
		if t.Name != "" {
			return t.Name // interface name
		}
		return "any"
	}
	return "any"
}

func (g *GoBackend) autoImport(path string) {
	if _, ok := g.imports[path]; !ok {
		g.imports[path] = ""
	}
}

// ---------------------------------------------------------------------------
// Declaration emission
// ---------------------------------------------------------------------------

func (g *GoBackend) emitTypeDef(td *LTypeDef) {
	name := g.visName(td.Name, td.IsExported)
	g.writef("type %s = %s\n\n", name, g.goType(td.Type))
}

func (g *GoBackend) emitStructDecl(s *LStructDecl) {
	name := g.visName(s.Name, s.IsExported)
	g.writef("type %s struct {\n", name)
	for _, f := range s.Fields {
		g.writef("\t%s %s\n", exportedFieldName(f.Name), g.goType(f.Type))
	}
	g.writef("}\n\n")
}

func (g *GoBackend) emitEnumDecl(e *LEnumDecl) {
	name := g.visName(e.Name, e.IsExported)

	// Interface type
	g.writef("type %s interface {\n", name)
	g.writef("\tis%s()\n", name)
	g.writef("}\n\n")

	// Variant structs
	for _, v := range e.Variants {
		vName := g.visName(v.Name, e.IsExported)
		g.writef("type %s struct {\n", vName)
		for _, f := range v.Fields {
			g.writef("\t%s %s\n", exportedFieldName(f.Name), g.goType(f.Type))
		}
		g.writef("}\n\n")
		g.writef("func (%s) is%s() {}\n\n", vName, name)
	}
}

func (g *GoBackend) emitClassDecl(c *LClassDecl) {
	name := g.visName(c.Name, c.IsExported)
	g.writef("type %s struct {\n", name)
	for _, f := range c.Fields {
		g.writef("\t%s %s\n", exportedFieldName(f.Name), g.goType(f.Type))
	}
	g.writef("}\n\n")
}

func (g *GoBackend) emitFuncDecl(f *LFuncDecl) {
	name := g.visName(f.Name, f.IsExported)

	// Build parameter list
	var params []string
	startIdx := 0
	if f.Receiver != "" {
		// Method — emit as (self *ReceiverType)
		startIdx = 1 // skip self param
		receiverType := g.visName(f.Receiver, true) // methods are always on exported types for now
		g.writef("func (self *%s) %s(", receiverType, name)
	} else {
		g.writef("func %s(", name)
	}

	for i := startIdx; i < len(f.Params); i++ {
		p := f.Params[i]
		params = append(params, fmt.Sprintf("%s %s", p.Name, g.goType(p.Type)))
	}
	g.buf.WriteString(strings.Join(params, ", "))
	g.buf.WriteString(")")

	// Return type
	if f.ReturnType != nil && f.ReturnType.Kind != LTyUnit {
		if f.ReturnType.Kind == LTyErrorResult {
			g.writef(" (%s, error)", g.goType(f.ReturnType.Elem))
		} else {
			g.writef(" %s", g.goType(f.ReturnType))
		}
	}

	g.writef(" {\n")
	g.indent++
	g.emitStmts(f.Body)
	g.indent--
	g.writef("}\n\n")
}

// ---------------------------------------------------------------------------
// Statement emission
// ---------------------------------------------------------------------------

func (g *GoBackend) emitStmts(stmts []LStmt) {
	for _, s := range stmts {
		g.emitStmt(&s)
	}
}

func (g *GoBackend) emitStmt(s *LStmt) {
	switch s.Kind {
	case LStmtTempDef:
		td := s.Data.(*LTempDef)
		g.writeIndent()
		g.writef("%s := ", g.tempName(td.ID))
		g.emitExpr(&td.Expr)
		g.writef("\n")

	case LStmtVarDecl:
		vd := s.Data.(*LVarDecl)
		g.writeIndent()
		if vd.Init != nil {
			g.writef("%s := ", vd.Name)
			g.emitValue(vd.Init)
		} else {
			g.writef("var %s %s", vd.Name, g.goType(vd.Type))
		}
		g.writef("\n")

	case LStmtAssign:
		a := s.Data.(*LAssign)
		g.writeIndent()
		g.writef("%s = ", a.Target)
		g.emitValue(&a.Value)
		g.writef("\n")

	case LStmtStructSet:
		ss := s.Data.(*LStructSet)
		g.writeIndent()
		g.emitValue(&ss.Receiver)
		g.writef(".%s = ", exportedFieldName(ss.Field))
		g.emitValue(&ss.Value)
		g.writef("\n")

	case LStmtClassSet:
		cs := s.Data.(*LClassSet)
		g.writeIndent()
		g.emitValue(&cs.Handle)
		g.writef(".%s = ", exportedFieldName(cs.Field))
		g.emitValue(&cs.Value)
		g.writef("\n")

	case LStmtIndexSet:
		is := s.Data.(*LIndexSet)
		g.writeIndent()
		g.emitValue(&is.Collection)
		g.writef("[")
		g.emitValue(&is.Index)
		g.writef("] = ")
		g.emitValue(&is.Value)
		g.writef("\n")

	case LStmtIf:
		ifData := s.Data.(*LIf)
		g.writeIndent()
		g.writef("if ")
		g.emitValue(&ifData.Cond)
		g.writef(" {\n")
		g.indent++
		g.emitStmts(ifData.Then)
		g.indent--
		if len(ifData.Else) > 0 {
			// Check if else is a single if (else-if chain)
			if len(ifData.Else) == 1 && ifData.Else[0].Kind == LStmtIf {
				g.writeIndent()
				g.writef("} else ")
				// Emit the inner if without leading indent
				innerIf := ifData.Else[0].Data.(*LIf)
				g.writef("if ")
				g.emitValue(&innerIf.Cond)
				g.writef(" {\n")
				g.indent++
				g.emitStmts(innerIf.Then)
				g.indent--
				if len(innerIf.Else) > 0 {
					g.writeIndent()
					g.writef("} else {\n")
					g.indent++
					g.emitStmts(innerIf.Else)
					g.indent--
				}
				g.writeIndent()
				g.writef("}\n")
			} else {
				g.writeIndent()
				g.writef("} else {\n")
				g.indent++
				g.emitStmts(ifData.Else)
				g.indent--
				g.writeIndent()
				g.writef("}\n")
			}
		} else {
			g.writeIndent()
			g.writef("}\n")
		}

	case LStmtWhile:
		w := s.Data.(*LWhile)
		g.writeIndent()
		if len(w.CondBlock) == 0 {
			// Simple condition
			g.writef("for ")
			g.emitValue(&w.CondVar)
			g.writef(" {\n")
		} else {
			// Complex condition: emit as for { condBlock; if !cond { break } ... }
			g.writef("for {\n")
			g.indent++
			g.emitStmts(w.CondBlock)
			g.writeIndent()
			g.writef("if !")
			g.emitValue(&w.CondVar)
			g.writef(" {\n")
			g.indent++
			g.writeIndent()
			g.writef("break\n")
			g.indent--
			g.writeIndent()
			g.writef("}\n")
			g.indent--
		}
		g.indent++
		g.emitStmts(w.Body)
		g.indent--
		g.writeIndent()
		g.writef("}\n")

	case LStmtFor:
		f := s.Data.(*LFor)
		g.writeIndent()
		if f.IndexVar != "" {
			g.writef("for %s, %s := range ", f.IndexVar, f.Var)
		} else {
			g.writef("for _, %s := range ", f.Var)
		}
		g.emitValue(&f.Collection)
		g.writef(" {\n")
		g.indent++
		g.emitStmts(f.Body)
		g.indent--
		g.writeIndent()
		g.writef("}\n")

	case LStmtSwitch:
		sw := s.Data.(*LSwitch)
		g.writeIndent()
		g.writef("switch ")
		g.emitValue(&sw.Tag)
		g.writef(" {\n")
		for _, c := range sw.Cases {
			g.writeIndent()
			if c.Tag == -1 {
				g.writef("default:\n")
			} else {
				g.writef("case %d:\n", c.Tag)
			}
			g.indent++
			g.emitStmts(c.Body)
			g.indent--
		}
		g.writeIndent()
		g.writef("}\n")

	case LStmtBlock:
		b := s.Data.(*LBlock)
		g.writeIndent()
		g.writef("{\n")
		g.indent++
		g.emitStmts(b.Stmts)
		g.indent--
		g.writeIndent()
		g.writef("}\n")

	case LStmtReturn:
		r := s.Data.(*LReturn)
		g.writeIndent()
		if len(r.Values) == 0 {
			g.writef("return\n")
		} else {
			g.writef("return ")
			for i, v := range r.Values {
				if i > 0 {
					g.writef(", ")
				}
				g.emitValue(&v)
			}
			g.writef("\n")
		}

	case LStmtBreak:
		g.writeIndent()
		g.writef("break\n")

	case LStmtContinue:
		g.writeIndent()
		g.writef("continue\n")

	case LStmtExpr:
		es := s.Data.(*LExprStmt)
		g.writeIndent()
		g.writef("_ = %s\n", g.tempName(es.TempID))

	case LStmtDefer:
		d := s.Data.(*LDefer)
		g.writeIndent()
		g.writef("defer func() {\n")
		g.indent++
		g.emitStmts(d.Body)
		g.indent--
		g.writeIndent()
		g.writef("}()\n")

	case LStmtSpawn:
		sp := s.Data.(*LSpawn)
		g.writeIndent()
		g.writef("go func() {\n")
		g.indent++
		g.emitStmts(sp.Body)
		g.indent--
		g.writeIndent()
		g.writef("}()\n")

	case LStmtLock:
		lk := s.Data.(*LLock)
		g.writeIndent()
		g.autoImport("sync")
		g.emitValue(&lk.Mutex)
		g.writef(".Lock()\n")
		g.writeIndent()
		g.writef("defer ")
		g.emitValue(&lk.Mutex)
		g.writef(".Unlock()\n")
		g.emitStmts(lk.Body)

	case LStmtSend:
		snd := s.Data.(*LSend)
		g.writeIndent()
		g.emitValue(&snd.Channel)
		g.writef(" <- ")
		g.emitValue(&snd.Value)
		g.writef("\n")

	case LStmtSelect:
		sel := s.Data.(*LSelect)
		g.writeIndent()
		g.writef("select {\n")
		for _, c := range sel.Cases {
			g.writeIndent()
			switch c.Kind {
			case LSelectDefault:
				g.writef("default:\n")
			case LSelectRecv:
				if c.Binding != "" {
					g.writef("case %s := <-", c.Binding)
				} else {
					g.writef("case <-")
				}
				g.emitValue(&c.Channel)
				g.writef(":\n")
			case LSelectSend:
				g.writef("case ")
				g.emitValue(&c.Channel)
				g.writef(" <- ")
				g.emitValue(&c.Value)
				g.writef(":\n")
			}
			g.indent++
			g.emitStmts(c.Body)
			g.indent--
		}
		g.writeIndent()
		g.writef("}\n")
	}
}

// ---------------------------------------------------------------------------
// Expression emission
// ---------------------------------------------------------------------------

func (g *GoBackend) emitExpr(e *LExpr) {
	switch e.Kind {
	case LExprBinOp:
		d := e.Data.(*LBinOpData)
		g.emitValue(&d.Left)
		g.writef(" %s ", goBinOp(d.Op))
		g.emitValue(&d.Right)

	case LExprUnOp:
		d := e.Data.(*LUnOpData)
		g.writef("%s", goUnOp(d.Op))
		g.emitValue(&d.Operand)

	case LExprCast:
		d := e.Data.(*LCastData)
		g.writef("%s(", g.goType(d.Target))
		g.emitValue(&d.Operand)
		g.writef(")")

	case LExprStructField:
		d := e.Data.(*LStructFieldData)
		g.emitValue(&d.Receiver)
		g.writef(".%s", exportedFieldName(d.Field))

	case LExprClassGet:
		d := e.Data.(*LClassGetData)
		g.emitValue(&d.Handle)
		g.writef(".%s", exportedFieldName(d.Field))

	case LExprIndexGet:
		d := e.Data.(*LIndexGetData)
		g.emitValue(&d.Collection)
		g.writef("[")
		g.emitValue(&d.Index)
		g.writef("]")

	case LExprSlice:
		d := e.Data.(*LSliceData)
		g.emitValue(&d.Collection)
		g.writef("[")
		if d.Low != nil {
			g.emitValue(d.Low)
		}
		g.writef(":")
		if d.High != nil {
			g.emitValue(d.High)
		}
		g.writef("]")

	case LExprCall:
		d := e.Data.(*LCallData)
		g.writef("%s(", d.Func)
		for i, arg := range d.Args {
			if i > 0 {
				g.writef(", ")
			}
			g.emitValue(&arg)
		}
		g.writef(")")

	case LExprBuiltin:
		d := e.Data.(*LBuiltinData)
		g.emitBuiltin(d)

	case LExprStructLit:
		d := e.Data.(*LStructLitData)
		typeName := g.goType(e.Type)
		if e.Type != nil && e.Type.Kind == LTySlice {
			// List literal
			g.writef("%s{", typeName)
			for i, f := range d.Fields {
				if i > 0 {
					g.writef(", ")
				}
				g.emitValue(&f.Value)
			}
			g.writef("}")
		} else {
			g.writef("%s{", typeName)
			for i, f := range d.Fields {
				if i > 0 {
					g.writef(", ")
				}
				g.writef("%s: ", exportedFieldName(f.Name))
				g.emitValue(&f.Value)
			}
			g.writef("}")
		}

	case LExprClassAlloc:
		d := e.Data.(*LClassAllocData)
		g.writef("&%s{}", d.Class)

	case LExprMakeSlice:
		elemType := g.goType(e.Type.Elem)
		g.writef("make([]%s, 0)", elemType)

	case LExprMakeMap:
		g.writef("make(%s)", g.goType(e.Type))

	case LExprMakeChannel:
		d := e.Data.(*LMakeChannelData)
		if d.BufSize != nil {
			g.writef("make(chan %s, ", g.goType(d.ElemType))
			g.emitValue(d.BufSize)
			g.writef(")")
		} else {
			g.writef("make(chan %s)", g.goType(d.ElemType))
		}

	case LExprWrapOptional:
		d := e.Data.(*LWrapOptionalData)
		// Create pointer: func() *T { v := val; return &v }()
		g.writef("func() %s { _v := ", g.goType(e.Type))
		g.emitValue(&d.Value)
		g.writef("; return &_v }()")

	case LExprUnwrapOptional:
		d := e.Data.(*LUnwrapOptionalData)
		g.writef("*")
		g.emitValue(&d.Value)

	case LExprIsNull:
		d := e.Data.(*LIsNullData)
		g.emitValue(&d.Value)
		g.writef(" == nil")

	case LExprVariantConstruct:
		d := e.Data.(*LVariantConstructData)
		if len(d.Fields) == 0 {
			g.writef("%s{}", d.Variant)
		} else {
			g.writef("%s{", d.Variant)
			for i, f := range d.Fields {
				if i > 0 {
					g.writef(", ")
				}
				g.emitValue(&f)
			}
			g.writef("}")
		}

	case LExprVariantTag:
		// Emit a type switch expression — this needs to be handled at the statement level
		g.writef("/* variant tag */")

	case LExprVariantData:
		d := e.Data.(*LVariantDataData)
		g.emitValue(&d.Value)
		g.writef(".(%s).%s", d.Variant, exportedFieldName(d.Field))

	case LExprExtractValue:
		// First element of multi-return — handled by caller context
		d := e.Data.(*LExtractValueData)
		g.emitValue(&d.Value)

	case LExprExtractError:
		d := e.Data.(*LExtractErrorData)
		g.emitValue(&d.Value)

	case LExprMakeResult:
		d := e.Data.(*LMakeResultData)
		g.emitValue(&d.Value)
		g.writef(", ")
		g.emitValue(&d.Err)

	case LExprFuncRef:
		d := e.Data.(*LFuncRefData)
		g.writef("%s", d.Name)

	case LExprFormat:
		d := e.Data.(*LFormatData)
		g.autoImport("fmt")
		var fmtStr strings.Builder
		var args []LValue
		for _, p := range d.Parts {
			if p.IsLiteral {
				fmtStr.WriteString(p.Text)
			} else {
				fmtStr.WriteString(p.Format)
				args = append(args, p.Value)
			}
		}
		g.writef("fmt.Sprintf(%q", fmtStr.String())
		for _, arg := range args {
			g.writef(", ")
			g.emitValue(&arg)
		}
		g.writef(")")
	}
}

func (g *GoBackend) emitBuiltin(d *LBuiltinData) {
	switch d.Name {
	case "println":
		g.autoImport("fmt")
		g.writef("fmt.Println(")
		g.emitArgs(d.Args)
		g.writef(")")
	case "print":
		g.autoImport("fmt")
		g.writef("fmt.Print(")
		g.emitArgs(d.Args)
		g.writef(")")
	case "len":
		g.writef("len(")
		g.emitArgs(d.Args)
		g.writef(")")
	case "append":
		g.writef("append(")
		g.emitArgs(d.Args)
		g.writef(")")
	case "isnull":
		if len(d.Args) > 0 {
			g.emitValue(&d.Args[0])
			g.writef(" == nil")
		}
	case "channel_receive":
		if len(d.Args) > 0 {
			g.writef("<-")
			g.emitValue(&d.Args[0])
		}
	case "channel_close":
		if len(d.Args) > 0 {
			g.writef("close(")
			g.emitValue(&d.Args[0])
			g.writef(")")
		}
	default:
		// Built-in methods: string_X, slice_X, map_X
		if strings.HasPrefix(d.Name, "string_") || strings.HasPrefix(d.Name, "slice_") || strings.HasPrefix(d.Name, "map_") {
			g.emitBuiltinMethod(d)
		} else {
			g.writef("%s(", d.Name)
			g.emitArgs(d.Args)
			g.writef(")")
		}
	}
}

func (g *GoBackend) emitBuiltinMethod(d *LBuiltinData) {
	parts := strings.SplitN(d.Name, "_", 2)
	if len(parts) != 2 || len(d.Args) == 0 {
		g.writef("/* unknown builtin %s */", d.Name)
		return
	}
	method := parts[1]
	recv := d.Args[0]
	args := d.Args[1:]

	switch method {
	case "contains":
		g.autoImport("strings")
		g.writef("strings.Contains(")
		g.emitValue(&recv)
		g.writef(", ")
		g.emitArgs(args)
		g.writef(")")
	case "split":
		g.autoImport("strings")
		g.writef("strings.Split(")
		g.emitValue(&recv)
		g.writef(", ")
		g.emitArgs(args)
		g.writef(")")
	case "trim":
		g.autoImport("strings")
		g.writef("strings.TrimSpace(")
		g.emitValue(&recv)
		g.writef(")")
	case "to_upper":
		g.autoImport("strings")
		g.writef("strings.ToUpper(")
		g.emitValue(&recv)
		g.writef(")")
	case "to_lower":
		g.autoImport("strings")
		g.writef("strings.ToLower(")
		g.emitValue(&recv)
		g.writef(")")
	case "push":
		// slice.push(x) → append(slice, x)
		g.writef("append(")
		g.emitValue(&recv)
		g.writef(", ")
		g.emitArgs(args)
		g.writef(")")
	case "get":
		// map.get(key)
		g.emitValue(&recv)
		g.writef("[")
		g.emitArgs(args)
		g.writef("]")
	case "has_key":
		if len(args) > 0 {
			g.writef("func() bool { _, ok := ")
			g.emitValue(&recv)
			g.writef("[")
			g.emitValue(&args[0])
			g.writef("]; return ok }()")
		}
	default:
		g.emitValue(&recv)
		g.writef(".%s(", method)
		g.emitArgs(args)
		g.writef(")")
	}
}

func (g *GoBackend) emitArgs(args []LValue) {
	for i, a := range args {
		if i > 0 {
			g.writef(", ")
		}
		g.emitValue(&a)
	}
}

// ---------------------------------------------------------------------------
// Value emission
// ---------------------------------------------------------------------------

func (g *GoBackend) emitValue(v *LValue) {
	switch v.Kind {
	case LValVar:
		g.writef("%s", v.Name)
	case LValTemp:
		g.writef("%s", g.tempName(v.TempID))
	case LValGlobal:
		g.writef("%s", v.Name)
	case LValLitInt:
		g.writef("%d", v.IntVal)
	case LValLitUint:
		g.writef("%d", v.UintVal)
	case LValLitFloat:
		g.writef("%g", v.FloatVal)
	case LValLitString:
		g.writef("%q", v.StrVal)
	case LValLitBool:
		if v.BoolVal {
			g.writef("true")
		} else {
			g.writef("false")
		}
	case LValLitNull:
		g.writef("nil")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (g *GoBackend) tempName(id int) string {
	return fmt.Sprintf("_t%d", id)
}

func (g *GoBackend) visName(name string, exported bool) string {
	if exported {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return strings.ToLower(name[:1]) + name[1:]
}

func exportedFieldName(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func goBinOp(op LBinOpKind) string {
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
	}
	return "?"
}

func goUnOp(op LUnOpKind) string {
	switch op {
	case LUnNeg:
		return "-"
	case LUnNot:
		return "!"
	case LUnBitNot:
		return "^"
	}
	return "?"
}

func (g *GoBackend) writef(format string, args ...interface{}) {
	if len(args) == 0 {
		g.buf.WriteString(format)
	} else {
		g.buf.WriteString(fmt.Sprintf(format, args...))
	}
}

func (g *GoBackend) writeIndent() {
	for i := 0; i < g.indent; i++ {
		g.buf.WriteByte('\t')
	}
}

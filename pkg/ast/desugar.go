package ast

// DesugarDefaultImpls extracts interface methods with bodies into top-level
// functions with relational where clauses. This must run before the checker.
//
//	interface Graph<G, N, E> {
//	  func G.nodes(self) -> [N]
//	  pub func count_edges(graph: G) -> i32 { ... }
//	}
//
// becomes:
//
//	pub func count_edges<G, N, E>(graph: G) -> i32 where Graph<G, N, E> { ... }
func DesugarDefaultImpls(file *File) {
	for bi := range file.Blocks {
		block := &file.Blocks[bi]
		for ii := range block.Interfaces {
			iface := &block.Interfaces[ii]
			var kept []FuncDecl
			for _, m := range iface.Methods {
				if m.Body != nil {
					// Extract as top-level function with interface type params + where clause
					fn := m
					fn.ReceiverType = "" // not a typed method anymore

					// Add interface type params
					for _, tp := range iface.TypeParams {
						fn.TypeParams = append(fn.TypeParams, TypeParam{
							Name:       tp.Name,
							Constraint: tp.Constraint,
						})
					}

					// Add relational where clause: where Graph<G, N, E>
					var typeArgs []TypeExpr
					for _, tp := range iface.TypeParams {
						typeArgs = append(typeArgs, TypeExpr{
							Kind: TypeNamed,
							Data: NamedType{Name: tp.Name},
						})
					}
					fn.Where = append(fn.Where, WhereClause{
						Constraint: iface.Name,
						TypeArgs:   typeArgs,
					})

					block.Functions = append(block.Functions, fn)
				} else {
					kept = append(kept, m)
				}
			}
			iface.Methods = kept
		}
	}
}

// DesugarInterfaceFields converts interface field declarations into getter/setter
// methods on the interface. Must run before DesugarRelations and DesugarDefaultImpls.
//
//	field P.first: C?
//
// becomes:
//
//	func P.first(self) -> C?
//	func P.set_first(mut self, val: C?)
func DesugarInterfaceFields(file *File) {
	for bi := range file.Blocks {
		block := &file.Blocks[bi]
		for ii := range block.Interfaces {
			iface := &block.Interfaces[ii]
			for _, fd := range iface.Fields {
				// Getter: func T.name(self) -> Type
				getter := FuncDecl{
					Name:         fd.Name,
					ReceiverType: fd.TypeParam,
					Params: []Param{
						{Name: "self", IsSelf: true},
					},
					ReturnType: &fd.Type,
					Span:       fd.Span,
				}
				iface.Methods = append(iface.Methods, getter)

				// Setter: func T.set_name(mut self, val: Type)
				setter := FuncDecl{
					Name:         "set_" + fd.Name,
					ReceiverType: fd.TypeParam,
					Params: []Param{
						{Name: "self", IsSelf: true, IsMut: true},
						{Name: "val", Type: fd.Type},
					},
					Span: fd.Span,
				}
				iface.Methods = append(iface.Methods, setter)
			}
		}
	}
}

// DesugarRelations processes relation declarations:
// 1. Injects default fields from the interface into concrete classes (with label prefixing)
// 2. Generates impl blocks with field bindings mapping interface getters to concrete fields
func DesugarRelations(file *File) {
	for bi := range file.Blocks {
		block := &file.Blocks[bi]

		// Build interface lookup: name -> InterfaceDecl
		ifaceMap := make(map[string]*InterfaceDecl)
		for ii := range block.Interfaces {
			ifaceMap[block.Interfaces[ii].Name] = &block.Interfaces[ii]
		}

		// Build class lookup: name -> index in block.Classes
		classIdx := make(map[string]int)
		for ci := range block.Classes {
			classIdx[block.Classes[ci].Name] = ci
		}

		for _, rel := range block.Relations {
			iface := ifaceMap[rel.Hint]
			if iface == nil || len(iface.Fields) == 0 {
				continue
			}

			if len(iface.TypeParams) < 2 {
				continue
			}

			// Map interface type params to concrete types from the relation
			typeMap := make(map[string]RelationSide) // type param name -> relation side
			typeMap[iface.TypeParams[0].Name] = rel.Parent
			typeMap[iface.TypeParams[1].Name] = rel.Child

			// Collect impl mappings for the auto-generated impl block
			var mappings []ImplMapping

			// For each interface field, inject into the appropriate concrete class
			for _, fd := range iface.Fields {
				side, ok := typeMap[fd.TypeParam]
				if !ok {
					continue
				}
				ci, ok := classIdx[side.TypeName]
				if !ok {
					continue
				}

				// Build field name with label prefix
				fieldName := fd.Name
				if side.Label != "" {
					fieldName = side.Label + "_" + fd.Name
				}

				// Rewrite type: replace type param references with concrete types
				fieldType := rewriteFieldType(fd.Type, iface.TypeParams, rel)

				block.Classes[ci].Fields = append(block.Classes[ci].Fields, Field{
					Name: fieldName,
					Type: fieldType,
					Span: fd.Span,
				})

				// Generate field binding for getter: T.name <-> ConcreteClass.prefixed_name
				mappings = append(mappings, ImplMapping{
					TypeParam:    fd.TypeParam,
					MethodName:   fd.Name,
					Kind:         ImplFieldBind,
					TargetClass:  side.TypeName,
					TargetMember: fieldName,
					Span:         fd.Span,
				})

				// Generate field binding for setter: T.set_name <-> ConcreteClass.prefixed_name
				mappings = append(mappings, ImplMapping{
					TypeParam:    fd.TypeParam,
					MethodName:   "set_" + fd.Name,
					Kind:         ImplFieldBind,
					TargetClass:  side.TypeName,
					TargetMember: fieldName,
					Span:         fd.Span,
				})
			}

			// Generate impl block with type args from the relation
			if len(mappings) > 0 {
				var typeArgs []TypeExpr
				typeArgs = append(typeArgs, TypeExpr{
					Kind: TypeNamed,
					Data: NamedType{Name: rel.Parent.TypeName},
				})
				typeArgs = append(typeArgs, TypeExpr{
					Kind: TypeNamed,
					Data: NamedType{Name: rel.Child.TypeName},
				})

				block.ImplBlocks = append(block.ImplBlocks, ImplBlock{
					InterfaceName: rel.Hint,
					TypeArgs:      typeArgs,
					Mappings:      mappings,
					Span:          rel.Span,
				})
			}
		}
	}
}

// rewriteFieldType replaces type parameter references in a field type with
// concrete class names from the relation.
func rewriteFieldType(te TypeExpr, typeParams []TypeParam, rel RelationDecl) TypeExpr {
	switch te.Kind {
	case TypeNamed:
		nt := te.Data.(NamedType)
		if len(typeParams) >= 1 && nt.Name == typeParams[0].Name {
			return TypeExpr{Kind: TypeNamed, Data: NamedType{Name: rel.Parent.TypeName}, Span: te.Span}
		}
		if len(typeParams) >= 2 && nt.Name == typeParams[1].Name {
			return TypeExpr{Kind: TypeNamed, Data: NamedType{Name: rel.Child.TypeName}, Span: te.Span}
		}
		return te
	case TypeOptional:
		ot := te.Data.(OptionalType)
		inner := rewriteFieldType(ot.Inner, typeParams, rel)
		return TypeExpr{Kind: TypeOptional, Data: OptionalType{Inner: inner}, Span: te.Span}
	case TypeSequence:
		st := te.Data.(SequenceType)
		elem := rewriteFieldType(st.Elem, typeParams, rel)
		return TypeExpr{Kind: TypeSequence, Data: SequenceType{Elem: elem}, Span: te.Span}
	default:
		return te
	}
}

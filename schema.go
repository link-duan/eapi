package eapi

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"github.com/gotomicro/eapi/spec"
	"github.com/gotomicro/eapi/tag"
	"github.com/iancoleman/strcase"
	"github.com/samber/lo"
)

const (
	MimeTypeJson           = "application/json"
	MimeApplicationXml     = "application/xml"
	MimeTypeXml            = "text/xml"
	MimeTypeFormData       = "multipart/form-data"
	MimeTypeFormUrlencoded = "application/x-www-form-urlencoded"
)

type SchemaBuilder struct {
	ctx         *Context
	contentType string
	stack       Stack[string]
	params      []*spec.SchemaRef
	typeParams  []*spec.TypeParam
}

func NewSchemaBuilder(ctx *Context, contentType string) *SchemaBuilder {
	return &SchemaBuilder{ctx: ctx, contentType: contentType}
}

func newSchemaBuilderWithStack(ctx *Context, contentType string, stack Stack[string]) *SchemaBuilder {
	return &SchemaBuilder{ctx: ctx, contentType: contentType, stack: stack}
}

func (s *SchemaBuilder) parseTypeDef(def *TypeDefinition) *spec.SchemaRef {
	schemaRef := s.parseTypeSpec(def.Spec)
	if schemaRef == nil {
		return nil
	}

	if len(def.Enums) > 0 {
		schema := spec.Unref(s.ctx.Doc(), schemaRef)
		ext := spec.NewExtendedEnumType(def.Enums...)
		schema.Value.ExtendedTypeInfo = ext
		for _, item := range def.Enums {
			schema.Value.Enum = append(schema.Value.Enum, item.Value)
		}
	}

	return schemaRef
}

func (s *SchemaBuilder) parseTypeSpec(t *ast.TypeSpec) *spec.SchemaRef {
	var typeParams []*spec.TypeParam
	if t.TypeParams != nil {
		for i, field := range t.TypeParams.List {
			for j, name := range field.Names {
				typeParams = append(typeParams, &spec.TypeParam{
					Index:      i + j,
					Name:       name.Name,
					Constraint: field.Type.(*ast.Ident).Name,
				})
			}
		}
	}

	schema := s.withTypeParams(typeParams).ParseExpr(t.Type)
	if schema == nil {
		return nil
	}
	if t.TypeParams != nil {
		schema.Value.WithExtendedType(spec.NewGenericExtendedType(typeParams))
	}

	comment := ParseComment(s.ctx.GetHeadingCommentOf(t.Pos()))
	schema.Value.Title = strcase.ToCamel(s.ctx.Package().Name + t.Name.Name)
	schema.Value.Description = strings.TrimSpace(comment.TrimPrefix(t.Name.Name))
	schema.Value.Deprecated = comment.Deprecated()
	return schema
}

func (s *SchemaBuilder) withTypeParams(params []*spec.TypeParam) *SchemaBuilder {
	s.typeParams = params
	return s
}

func (s *SchemaBuilder) withParams(params ...*spec.SchemaRef) *SchemaBuilder {
	res := *s
	for i, param := range params {
		param = param.Unref(s.ctx.Doc())
		ext := param.Value.ExtendedTypeInfo
		if ext == nil {
			continue
		}
		switch ext.Type {
		case spec.ExtendedTypeSpecific:
			// TODO
			panic("implement me")

		case spec.ExtendedTypeParam:
			params[i] = s.params[ext.TypeParam.Index]
		}
	}
	res.params = params
	return &res
}

func (s *SchemaBuilder) ParseExpr(expr ast.Expr) (schema *spec.SchemaRef) {
	switch expr := expr.(type) {
	case *ast.StructType:
		return s.parseStruct(expr)

	case *ast.StarExpr:
		return s.ParseExpr(expr.X)

	case *ast.Ident:
		return s.parseIdent(expr)

	case *ast.SelectorExpr:
		return s.ParseExpr(expr.Sel)

	case *ast.MapType:
		value := s.ParseExpr(expr.Value)
		return spec.NewSchemaRef(
			"",
			spec.NewObjectSchema().
				WithExtendedType(spec.NewMapExtendedType(
					s.ParseExpr(expr.Key),
					value,
				)).
				WithAdditionalProperties(value),
		)

	case *ast.ArrayType:
		return spec.ArrayProperty(s.ParseExpr(expr.Elt))

	case *ast.SliceExpr:
		return spec.ArrayProperty(s.ParseExpr(expr.X))

	case *ast.UnaryExpr:
		return s.ParseExpr(expr.X)

	case *ast.CompositeLit:
		return s.ParseExpr(expr.Type)

	case *ast.InterfaceType:
		return spec.NewSchemaRef("", spec.NewObjectSchema().WithDescription("Any Type").WithExtendedType(spec.NewAnyExtendedType()))

	case *ast.CallExpr:
		return s.parseCallExpr(expr)

	case *ast.IndexExpr:
		return s.parseIndexExpr(expr)

	case *ast.IndexListExpr:
		return s.parseIndexListExpr(expr)
	}

	// TODO
	return nil
}

func (s *SchemaBuilder) parseStruct(expr *ast.StructType) *spec.SchemaRef {
	schema := spec.NewObjectSchema()
	schema.Properties = make(spec.Schemas)

	var contentType = s.contentType
	if s.contentType == "" {
		contentType = "application/json" // fallback to json
	}

	for _, field := range expr.Fields.List {
		comment := s.parseCommentOfField(field)
		if comment.Ignore() {
			continue // ignored field
		}

		if len(field.Names) == 0 { // type composition
			fieldSchema := s.ParseExpr(field.Type)
			if fieldSchema != nil {
				// merge properties
				fieldSchema = spec.Unref(s.ctx.Doc(), fieldSchema)
				if fieldSchema.Value != nil {
					for name, value := range fieldSchema.Value.Properties {
						schema.Properties[name] = value
					}
				}
			}
		}

		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			fieldSchema := s.ParseExpr(field.Type)
			if fieldSchema == nil {
				fmt.Printf("unknown field type %s at %s\n", name.Name, s.ctx.LineColumn(field.Type.Pos()))
				continue
			}
			propName := s.getPropName(name.Name, field, contentType)
			if propName == "-" { // ignore
				continue
			}

			if comment != nil {
				comment.ApplyToSchema(fieldSchema)
				if comment.Required() {
					schema.Required = append(schema.Required, propName)
				}
			}
			schema.Properties[propName] = fieldSchema
		}
	}

	return spec.NewSchemaRef("", schema)
}

func (s *SchemaBuilder) parseIdent(expr *ast.Ident) *spec.SchemaRef {
	t := s.ctx.Package().TypesInfo.TypeOf(expr)
	switch t := t.(type) {
	case *types.Basic:
		return s.basicType(t.Name())
	case *types.Interface:
		return spec.NewSchemaRef("", spec.NewSchema().
			WithType("object").
			WithDescription("Any Type").
			WithExtendedType(spec.NewAnyExtendedType()))
	case *types.TypeParam:
		return spec.NewTypeParamSchema(s.typeParams[t.Index()]).NewRef()
	}

	// 检查是否是常用类型
	schema := s.commonUsedType(t)
	if schema != nil {
		return schema
	}

	return s.parseType(t)
}

var commonTypes = map[string]*spec.Schema{
	"time.Time": spec.NewSchema().WithType("string").WithFormat("datetime"),
	"encoding/json.RawMessage": spec.NewSchema().
		WithType("object").
		WithDescription("Any Json Type").
		WithExtendedType(spec.NewAnyExtendedType()),
	"json.RawMessage": spec.NewSchema().
		WithType("object").
		WithDescription("Any Json Type").
		WithExtendedType(spec.NewAnyExtendedType()),
}

func (s *SchemaBuilder) commonUsedType(t types.Type) *spec.SchemaRef {
	switch t := t.(type) {
	case *types.Named:
		typeName := t.Obj().Pkg().Path() + "." + t.Obj().Name()
		commonType, ok := commonTypes[typeName]
		if !ok {
			return nil
		}
		return spec.NewSchemaRef("", commonType.Clone())

	case *types.Pointer:
		return s.commonUsedType(t.Elem())
	}

	return nil
}

func (s *SchemaBuilder) parseSelectorExpr(expr *ast.SelectorExpr) *spec.SchemaRef {
	return s.ParseExpr(expr.Sel)
}

func (s *SchemaBuilder) getPropName(fieldName string, field *ast.Field, contentType string) (propName string) {
	if field.Tag == nil {
		return fieldName
	}

	tags := tag.Parse(field.Tag.Value)
	var tagValue string
	switch contentType {
	case MimeTypeJson:
		tagValue = tags["json"]
	case MimeTypeXml, MimeApplicationXml:
		tagValue = tags["xml"]
	case MimeTypeFormData, MimeTypeFormUrlencoded:
		tagValue = tags["form"]
	}
	if tagValue == "" {
		return fieldName
	}

	propName, _, _ = strings.Cut(tagValue, ",")
	return
}

func (s *SchemaBuilder) basicType(name string) *spec.SchemaRef {
	switch name {
	case "uint", "int", "uint8", "int8", "uint16", "int16",
		"uint32", "int32", "uint64", "int64":
		return spec.NewSchemaRef("", spec.NewIntegerSchema())
	case "byte", "rune":
		return spec.NewSchemaRef("", spec.NewBytesSchema())
	case "float32", "float64":
		return spec.NewSchemaRef("", spec.NewFloat64Schema())
	case "bool":
		return spec.NewSchemaRef("", spec.NewBoolSchema())
	case "string":
		return spec.NewSchemaRef("", spec.NewStringSchema())
	}

	return nil
}

func (s *SchemaBuilder) parseType(t types.Type) *spec.SchemaRef {
	switch t := t.(type) {
	case *types.Slice:
		return spec.ArrayProperty(s.parseType(t.Elem()))
	case *types.Array:
		return spec.ArrayProperty(s.parseType(t.Elem()))
	}

	def := s.ctx.ParseType(t)
	typeDef, ok := def.(*TypeDefinition)
	if !ok {
		return nil
	}
	if lo.Contains(s.stack, typeDef.Key()) {
		return spec.RefSchema(typeDef.RefKey())
	}

	_, ok = s.ctx.Doc().Components.Schemas[typeDef.ModelKey()]
	if ok {
		return spec.RefSchema(typeDef.RefKey())
	}

	s.stack.Push(typeDef.Key())
	defer s.stack.Pop()

	payloadSchema := newSchemaBuilderWithStack(s.ctx.WithPackage(typeDef.pkg).WithFile(typeDef.file), s.contentType, append(s.stack, typeDef.Key())).
		parseTypeDef(typeDef)
	if payloadSchema == nil {
		return nil
	}
	s.ctx.Doc().Components.Schemas[typeDef.ModelKey()] = payloadSchema

	return spec.RefSchema(typeDef.RefKey())
}

func (s *SchemaBuilder) parseCommentOfField(field *ast.Field) *Comment {
	// heading comment
	if field.Doc != nil && len(field.Doc.List) > 0 {
		return ParseComment(field.Doc)
	}

	// parse trailing comment
	commentGroup := s.ctx.GetTrailingCommentOf(field.Pos())
	return ParseComment(commentGroup)
}

func (s *SchemaBuilder) parseCallExpr(expr *ast.CallExpr) *spec.SchemaRef {
	typeName, method, err := s.ctx.GetCallInfo(expr)
	if err != nil {
		return nil
	}

	commonType, ok := commonTypes[typeName+"."+method]
	if ok {
		return spec.NewSchemaRef("", commonType.Clone())
	}

	def := s.ctx.GetDefinition(typeName, method)
	if def == nil {
		fmt.Printf("unknown type/function %s.%s at %s\n", typeName, method, s.ctx.LineColumn(expr.Pos()))
		return nil
	}

	switch def := def.(type) {
	case *FuncDefinition:
		if def.Decl.Type.Results.NumFields() == 0 {
			return nil
		}
		ret := def.Decl.Type.Results.List[0]
		return newSchemaBuilderWithStack(s.ctx.WithPackage(def.pkg).WithFile(def.file), s.contentType, append(s.stack, def.Key())).
			ParseExpr(ret.Type)

	case *TypeDefinition:
		return newSchemaBuilderWithStack(s.ctx.WithPackage(def.pkg).WithFile(def.file), s.contentType, append(s.stack, def.Key())).
			parseTypeSpec(def.Spec)

	default:
		return nil
	}
}

func (s *SchemaBuilder) parseIndexExpr(expr *ast.IndexExpr) *spec.SchemaRef {
	paramType := s.ParseExpr(expr.Index)
	genericType := s.ParseExpr(expr.X)
	if genericType == nil {
		return nil
	}

	def := s.ctx.ParseType(s.ctx.Package().TypesInfo.TypeOf(expr.X)).(*TypeDefinition)
	typeKey := def.ModelKey() + "[" + s.getTypeKey(expr.Index) + "]"

	_, exists := s.ctx.Doc().Components.Schemas[typeKey]
	if !exists {
		// specialization
		// 这里要考虑类型参数引用了自身的情况，比如 GType[GType[int]]
		schemaRef := s.withParams(paramType).specializeGenericType(genericType)
		// TODO 暂时不处理泛型类型元信息
		//schemaRef.Value.WithExtendedType(spec.NewSpecificExtendType(genericType, paramType))
		s.ctx.Doc().Components.Schemas[typeKey] = schemaRef
	}

	return spec.NewSchemaRef("#/components/schemas/"+typeKey, nil)
}

func (s *SchemaBuilder) parseIndexListExpr(expr *ast.IndexListExpr) *spec.SchemaRef {
	// TODO

	//var params []*spec.SchemaRef
	//for _, param := range expr.Indices {
	//	params = append(params, s.ParseExpr(param))
	//}
	//genericType := s.ParseExpr(expr.X)
	//return spec.NewSchemaRef("", spec.NewObjectSchema().WithExtendedType(
	//	spec.NewSpecificExtendType(genericType, params...),
	//))

	return spec.NewSchema().WithDescription("TODO").NewRef()
}

func (s *SchemaBuilder) getTypeKey(expr ast.Expr) string {
	t := s.ctx.Package().TypesInfo.TypeOf(expr)
	switch t := t.(type) {
	case *types.Basic:
		return t.Name()
	default:
		def := s.ctx.ParseType(t)
		if def == nil {
			fmt.Printf("unknown type at %s\n", s.ctx.LineColumn(expr.Pos()))
			return ""
		}
		return def.(*TypeDefinition).ModelKey()
	}
}

func (s *SchemaBuilder) specializeGenericType(genericType *spec.SchemaRef) *spec.SchemaRef {
	schemaRef := genericType.Unref(s.ctx.Doc())
	schema := schemaRef.Value.Clone()
	ext := schema.ExtendedTypeInfo
	if ext == nil || ext.Type != spec.ExtendedTypeGeneric {
		return genericType
	}

	for key, property := range schema.Properties {
		property = property.Unref(s.ctx.Doc()).Clone()
		ext := property.Value.ExtendedTypeInfo
		if ext == nil {
			continue
		}
		switch ext.Type {
		case spec.ExtendedTypeParam:
			schema.Properties[key] = s.params[ext.TypeParam.Index]

		case spec.ExtendedTypeSpecific:
			schema.Properties[key] = s.withParams(ext.SpecificType.Params...).specializeGenericType(ext.SpecificType.Type)

		default:
			continue
		}
	}

	schema.ExtendedTypeInfo = nil
	schemaRef.Value = schema
	return schemaRef
}

package spec

type ExtendedType string

const (
	ExtendedTypeMap      ExtendedType = "map"
	ExtendedTypeAny      ExtendedType = "any"
	ExtendedTypeEnum     ExtendedType = "enum"
	ExtendedTypeSpecific ExtendedType = "specific"
	ExtendedTypeGeneric  ExtendedType = "generic"
	ExtendedTypeParam    ExtendedType = "param"
)

type ExtendedTypeInfo struct {
	Type ExtendedType

	MapKey   *SchemaRef
	MapValue *SchemaRef

	EnumItems []*ExtendedEnumItem

	SpecificType *SpecificType
	GenericType  *GenericType

	TypeParam *TypeParam
}

func NewExtendedEnumType(items ...*ExtendedEnumItem) *ExtendedTypeInfo {
	return &ExtendedTypeInfo{
		Type:      ExtendedTypeEnum,
		MapValue:  nil,
		EnumItems: items,
	}
}

func NewAnyExtendedType() *ExtendedTypeInfo {
	return &ExtendedTypeInfo{Type: ExtendedTypeAny}
}

func NewMapExtendedType(key, value *SchemaRef) *ExtendedTypeInfo {
	return &ExtendedTypeInfo{
		Type:     ExtendedTypeMap,
		MapKey:   key,
		MapValue: value,
	}
}

type ExtendedEnumItem struct {
	Key         string      `json:"key"`
	Value       interface{} `json:"value"`
	Description string      `json:"description"`
}

func NewExtendEnumItem(key string, value interface{}, description string) *ExtendedEnumItem {
	return &ExtendedEnumItem{
		Key:         key,
		Value:       value,
		Description: description,
	}
}

type SpecificType struct {
	Params []*SchemaRef
	Type   *SchemaRef
}

func NewSpecificExtendType(genericType *SchemaRef, params ...*SchemaRef) *ExtendedTypeInfo {
	return &ExtendedTypeInfo{
		Type:         ExtendedTypeSpecific,
		SpecificType: &SpecificType{Params: params, Type: genericType},
	}
}

type GenericType struct {
	TypeParams []*TypeParam
}

func NewGenericExtendedType(typeParams []*TypeParam) *ExtendedTypeInfo {
	return &ExtendedTypeInfo{Type: ExtendedTypeGeneric, GenericType: &GenericType{TypeParams: typeParams}}
}

type TypeParam struct {
	Index      int
	Name       string
	Constraint string
}

func NewTypeParamExtendedType(param *TypeParam) *ExtendedTypeInfo {
	return &ExtendedTypeInfo{Type: ExtendedTypeParam, TypeParam: param}
}

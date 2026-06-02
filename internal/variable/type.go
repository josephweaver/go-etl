package variable

import "fmt"

type Kind string

const (
	KindString   Kind = "string"
	KindInt      Kind = "int"
	KindBool     Kind = "bool"
	KindDatetime Kind = "datetime"
	KindPath     Kind = "path"
	KindList     Kind = "list"
	KindObject   Kind = "object"
)

type Type struct {
	Kind    Kind
	Element *Type
}

var (
	TypeString   = Type{Kind: KindString}
	TypeInt      = Type{Kind: KindInt}
	TypeBool     = Type{Kind: KindBool}
	TypeDatetime = Type{Kind: KindDatetime}
	TypePath     = Type{Kind: KindPath}
	TypeObject   = Type{Kind: KindObject}
)

func TypeList(element Type) Type {
	return Type{Kind: KindList, Element: &element}
}

func (t Type) Valid() bool {
	switch t.Kind {
	case KindString,
		KindInt,
		KindBool,
		KindDatetime,
		KindPath,
		KindObject:
		return t.Element == nil
	case KindList:
		if t.Element == nil || t.Element.Kind == KindList {
			return false
		}
		return t.Element.Valid()
	default:
		return false
	}
}

func (t Type) String() string {
	if t.Kind == KindList {
		if t.Element == nil {
			return "list[<nil>]"
		}
		return fmt.Sprintf("list[%s]", t.Element.String())
	}

	if t.Kind == "" {
		return "<empty>"
	}

	return string(t.Kind)
}

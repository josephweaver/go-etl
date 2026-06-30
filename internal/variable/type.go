package variable

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
	Kind Kind
}

var (
	TypeString   = Type{Kind: KindString}
	TypeInt      = Type{Kind: KindInt}
	TypeBool     = Type{Kind: KindBool}
	TypeDatetime = Type{Kind: KindDatetime}
	TypePath     = Type{Kind: KindPath}
	TypeList     = Type{Kind: KindList}
	TypeObject   = Type{Kind: KindObject}
)

func (t Type) Valid() bool {
	switch t.Kind {
	case KindString,
		KindInt,
		KindBool,
		KindDatetime,
		KindPath,
		KindList,
		KindObject:
		return true
	default:
		return false
	}
}

func (t Type) String() string {
	if t.Kind == "" {
		return "<empty>"
	}

	return string(t.Kind)
}

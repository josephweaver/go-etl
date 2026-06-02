package variable

import "fmt"

type Scope map[string]Variable

type Set struct {
	merged Scope
	scopes map[Namespace]Scope
}

func Merge(scopes ...Scope) Scope {
	merged := make(Scope)

	for _, scope := range scopes {
		for key, variable := range scope {
			merged[key] = variable
		}
	}

	return merged
}

func NewScope(variables ...Variable) (Scope, error) {
	scope := make(Scope)

	for _, variable := range variables {
		if err := variable.Validate(); err != nil {
			return nil, err
		}

		if _, ok := scope[variable.Name.Key]; ok {
			return nil, fmt.Errorf("duplicate variable key: %s", variable.Name.Key)
		}

		scope[variable.Name.Key] = variable
	}

	return scope, nil
}

func NewSet(scopes ...Scope) Set {
	set := Set{
		merged: Merge(scopes...),
		scopes: make(map[Namespace]Scope),
	}

	for _, scope := range scopes {
		for key, variable := range scope {
			namespace := variable.Name.Namespace
			if set.scopes[namespace] == nil {
				set.scopes[namespace] = make(Scope)
			}
			set.scopes[namespace][key] = variable
		}
	}

	return set
}

func (s Set) Lookup(key string) (Variable, bool) {
	variable, ok := s.merged[key]
	return variable, ok
}

func (s Set) LookupName(name Name) (Variable, bool) {
	scope, ok := s.scopes[name.Namespace]
	if !ok {
		return Variable{}, false
	}

	variable, ok := scope[name.Key]
	return variable, ok
}

func (s Set) LookupReference(reference Reference) (Variable, bool) {
	if reference.Qualified {
		return s.LookupName(reference.Name)
	}

	return s.Lookup(reference.Name.Key)
}

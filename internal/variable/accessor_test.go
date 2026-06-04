package variable

import "testing"

func TestApplyAccessor(t *testing.T) {
	list, err := ResolvedList(TypeObject, []ResolvedValue{
		ResolvedObject(map[string]ResolvedValue{
			"year": {Type: TypeInt, Value: 2024},
		}),
		ResolvedObject(map[string]ResolvedValue{
			"year": {Type: TypeInt, Value: 2025},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	object := ResolvedObject(map[string]ResolvedValue{
		"items": list,
	})

	value, err := ApplyAccessor(object, ".items[1].year")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Type != TypeInt {
		t.Fatalf("unexpected type: %s", value.Type)
	}

	if value.Value != 2025 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestApplyAccessorStartsWithIndex(t *testing.T) {
	list, err := ResolvedList(TypeObject, []ResolvedValue{
		ResolvedObject(map[string]ResolvedValue{
			"year": {Type: TypeInt, Value: 2024},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	value, err := ApplyAccessor(list, "[0].year")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Value != 2024 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestApplyAccessorRejectsInvalidScalarChain(t *testing.T) {
	object := ResolvedObject(map[string]ResolvedValue{
		"year": {Type: TypeInt, Value: 2025},
	})

	tests := []string{
		"",
		"year",
		".year[*]",
		".year[0",
	}

	for _, accessor := range tests {
		t.Run(accessor, func(t *testing.T) {
			if _, err := ApplyAccessor(object, accessor); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestApplyFieldAccessor(t *testing.T) {
	object := ResolvedObject(map[string]ResolvedValue{
		"year": {Type: TypeInt, Value: 2025},
	})

	value, err := ApplyFieldAccessor(object, ".year")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Type != TypeInt {
		t.Fatalf("unexpected type: %s", value.Type)
	}

	if value.Value != 2025 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestApplyFieldAccessorRejectsInvalidAccessor(t *testing.T) {
	object := ResolvedObject(map[string]ResolvedValue{
		"year": {Type: TypeInt, Value: 2025},
	})

	tests := []string{
		"year",
		".",
		".year.month",
		".items[0]",
	}

	for _, accessor := range tests {
		t.Run(accessor, func(t *testing.T) {
			if _, err := ApplyFieldAccessor(object, accessor); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestApplyFieldAccessorRejectsNonObject(t *testing.T) {
	value := ResolvedValue{Type: TypeInt, Value: 2025}

	if _, err := ApplyFieldAccessor(value, ".year"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestApplyFieldAccessorRejectsMissingField(t *testing.T) {
	object := ResolvedObject(map[string]ResolvedValue{
		"year": {Type: TypeInt, Value: 2025},
	})

	if _, err := ApplyFieldAccessor(object, ".missing"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestApplyIndexAccessor(t *testing.T) {
	list, err := ResolvedList(TypeInt, []ResolvedValue{
		{Type: TypeInt, Value: 2024},
		{Type: TypeInt, Value: 2025},
	})
	if err != nil {
		t.Fatal(err)
	}

	value, err := ApplyIndexAccessor(list, "[1]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Type != TypeInt {
		t.Fatalf("unexpected type: %s", value.Type)
	}

	if value.Value != 2025 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestApplyIndexAccessorRejectsInvalidAccessor(t *testing.T) {
	list, err := ResolvedList(TypeInt, []ResolvedValue{
		{Type: TypeInt, Value: 2024},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []string{
		"0",
		"[]",
		"[first]",
		"[0",
	}

	for _, accessor := range tests {
		t.Run(accessor, func(t *testing.T) {
			if _, err := ApplyIndexAccessor(list, accessor); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestApplyIndexAccessorRejectsNonList(t *testing.T) {
	value := ResolvedValue{Type: TypeInt, Value: 2025}

	if _, err := ApplyIndexAccessor(value, "[0]"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestApplyIndexAccessorRejectsOutOfRange(t *testing.T) {
	list, err := ResolvedList(TypeInt, []ResolvedValue{
		{Type: TypeInt, Value: 2024},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyIndexAccessor(list, "[1]"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestApplyFanOutAccessor(t *testing.T) {
	list, err := ResolvedList(TypeInt, []ResolvedValue{
		{Type: TypeInt, Value: 2024},
		{Type: TypeInt, Value: 2025},
	})
	if err != nil {
		t.Fatal(err)
	}

	values, err := ApplyFanOutAccessor(list, "[*]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(values) != 2 {
		t.Fatalf("unexpected value count: %d", len(values))
	}

	if values[0].Value != 2024 {
		t.Fatalf("unexpected first value: %#v", values[0].Value)
	}
}

func TestApplyFanOutAccessorRejectsInvalidAccessor(t *testing.T) {
	list, err := ResolvedList(TypeInt, []ResolvedValue{
		{Type: TypeInt, Value: 2024},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyFanOutAccessor(list, "[0]"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestApplyFanOutAccessorRejectsNonList(t *testing.T) {
	value := ResolvedValue{Type: TypeInt, Value: 2025}

	if _, err := ApplyFanOutAccessor(value, "[*]"); err == nil {
		t.Fatal("expected an error")
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"goetl/internal/model"
	"goetl/internal/variable"
)

type OperationContext struct {
	WorkItem  model.WorkItem
	Public    map[string]variable.ResolvedValue
	Sensitive map[string]SensitiveValue
	Redactor  *Redactor
	Logger    SafeLogger
}

type SafeLogger interface {
	Log(message string) error
}

type workerSafeLogger struct {
	redactor *Redactor
	log      func(string) error
}

func (l workerSafeLogger) Log(message string) error {
	if l.redactor != nil {
		message = l.redactor.RedactString(message)
	}
	if l.log == nil {
		return nil
	}
	return l.log(message)
}

func (ctx OperationContext) String() string {
	return fmt.Sprintf("OperationContext{work_item:%s type:%s public:%d sensitive:%d}", ctx.WorkItem.ID, ctx.WorkItem.Type, len(ctx.Public), len(ctx.Sensitive))
}

func (ctx OperationContext) GoString() string {
	return ctx.String()
}

func (ctx OperationContext) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		WorkItemID      string   `json:"work_item_id"`
		WorkItemType    string   `json:"work_item_type"`
		PublicNames     []string `json:"public_names"`
		SensitiveNames  []string `json:"sensitive_names"`
		SensitiveValues string   `json:"sensitive_values"`
	}{
		WorkItemID:      ctx.WorkItem.ID,
		WorkItemType:    string(ctx.WorkItem.Type),
		PublicNames:     sortedKeys(ctx.Public),
		SensitiveNames:  sortedKeys(ctx.Sensitive),
		SensitiveValues: sensitiveValueRedactedLabel,
	})
}

func (w Worker) operationContext(ctx context.Context, item model.WorkItem, sensitiveNeeds []string) (OperationContext, error) {
	envelope := item.ExecutionEnvelope
	if envelope == nil {
		built, err := model.NewExecutionEnvelope(item)
		if err != nil {
			return OperationContext{}, err
		}
		envelope = &built
	}

	public, err := operationPublicValues(envelope.Variables.Public)
	if err != nil {
		return OperationContext{}, err
	}

	redactor := NewRedactor()
	operation := OperationContext{
		WorkItem:  item,
		Public:    public,
		Sensitive: map[string]SensitiveValue{},
		Redactor:  redactor,
	}
	operation.Logger = workerSafeLogger{redactor: redactor, log: w.log}

	resolver := WorkerEnvProtectedValueResolver{}
	for _, name := range sensitiveNeeds {
		ref, ok := envelope.Variables.ProtectedRefs[name]
		if !ok {
			continue
		}
		value, err := resolver.ResolveProtectedValue(ctx, ref)
		if err != nil {
			return OperationContext{}, fmt.Errorf("resolve sensitive value %s: %w", name, err)
		}
		operation.Sensitive[name] = value
		operation.Redactor.Register(value)
	}

	return operation, nil
}

func operationPublicValues(values map[string]model.ExecutionEnvelopePublicValue) (map[string]variable.ResolvedValue, error) {
	public := make(map[string]variable.ResolvedValue, len(values))
	for name, value := range values {
		public[name] = variable.ResolvedValue{
			Type:  operationValueType(value.Type),
			Value: value.Value,
		}
	}
	return public, nil
}

func operationValueType(raw string) variable.Type {
	switch raw {
	case "string":
		return variable.TypeString
	case "path":
		return variable.TypePath
	case "int":
		return variable.TypeInt
	case "bool":
		return variable.TypeBool
	case "datetime":
		return variable.TypeDatetime
	case "list":
		return variable.TypeList
	case "object":
		return variable.TypeObject
	default:
		return variable.Type{Kind: variable.Kind(raw)}
	}
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

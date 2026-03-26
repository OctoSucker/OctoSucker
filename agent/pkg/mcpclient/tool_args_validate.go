package mcpclient

import (
	"encoding/json"
	"fmt"
	"slices"
)

func propertyKeysFromSchema(schema map[string]any) (allowed map[string]struct{}, definite bool) {
	rawProps, has := schema["properties"]
	if !has || rawProps == nil {
		if typ, _ := schema["type"].(string); typ == "object" {
			if ap, ok := schema["additionalProperties"].(bool); ok && ap {
				return nil, false
			}
			// No property list: treat as empty object (MCP tools with no inputs). JSON Schema
			// would default additionalProperties elsewhere; for planner we require explicit
			// additionalProperties:true to allow arbitrary keys.
			return map[string]struct{}{}, true
		}
		return nil, false
	}
	raw, err := json.Marshal(rawProps)
	if err != nil {
		return nil, false
	}
	var propObj map[string]any
	if err := json.Unmarshal(raw, &propObj); err != nil {
		return nil, false
	}
	out := make(map[string]struct{}, len(propObj))
	for k := range propObj {
		out[k] = struct{}{}
	}
	return out, true
}

func ValidateToolArguments(toolName string, args map[string]any, schema any) error {
	sm := schemaAsMap(schema)
	if sm == nil {
		return fmt.Errorf("missing or unreadable input schema")
	}
	allowed, ok := propertyKeysFromSchema(sm)
	if !ok {
		return nil
	}
	if args == nil {
		args = map[string]any{}
	}
	if len(allowed) == 0 && len(args) > 0 {
		keys := argKeysSorted(args)
		return fmt.Errorf("accepts no properties; arguments had keys %v", keys)
	}
	var disallowed []string
	for k := range args {
		if _, ok := allowed[k]; !ok {
			disallowed = append(disallowed, k)
		}
	}
	if len(disallowed) == 0 {
		return nil
	}
	slices.Sort(disallowed)
	allowedList := keysFromSet(allowed)
	slices.Sort(allowedList)
	return fmt.Errorf("disallowed argument keys %v (allowed: %v)", disallowed, allowedList)
}

func schemaAsMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	if m, ok := schema.(map[string]any); ok {
		return m
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

func argKeysSorted(args map[string]any) []string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func keysFromSet(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

package jira

import "encoding/json"

func cloneJSONMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneJSONValue(value)
	}
	return out
}

func cloneJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneJSONMap(v)
	case map[string]string:
		out := make(map[string]string, len(v))
		for key, value := range v {
			out[key] = value
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, value := range v {
			out[i] = cloneJSONValue(value)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(v))
		for i, value := range v {
			out[i] = cloneJSONMap(value)
		}
		return out
	case []map[string]string:
		out := make([]map[string]string, len(v))
		for i, value := range v {
			out[i] = cloneJSONValue(value).(map[string]string)
		}
		return out
	case []string:
		return append([]string(nil), v...)
	case []json.RawMessage:
		out := make([]json.RawMessage, len(v))
		for i, value := range v {
			out[i] = append(json.RawMessage(nil), value...)
		}
		return out
	case map[string][]any:
		out := make(map[string][]any, len(v))
		for key, value := range v {
			out[key] = cloneJSONValue(value).([]any)
		}
		return out
	case map[string][]string:
		out := make(map[string][]string, len(v))
		for key, value := range v {
			out[key] = append([]string(nil), value...)
		}
		return out
	case json.RawMessage:
		return append(json.RawMessage(nil), v...)
	default:
		return value
	}
}

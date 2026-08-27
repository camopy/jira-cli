package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli/cache/registry"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

type fieldMetadata struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func loadCustomFields(ctx context.Context, entries []config.TUICustomField, cacheKey string, client *jira.Client) ([]core.CustomField, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	resource, _ := registry.ByName("fields")
	var (
		data    json.RawMessage
		loadErr error
	)
	if needsFieldMetadata(entries) {
		if client == nil {
			entry, ok, err := cache.ReadCachedOrEmpty(cacheKey, resource.Name)
			switch {
			case err != nil:
				loadErr = err
			case ok:
				data = entry.Data
			default:
				loadErr = errors.New("custom field metadata is unavailable; configure customfield_NNNNN IDs or authenticate Jira")
			}
		} else {
			data, _, _, _, loadErr = cmdutil.CacheReadOrFetch(cacheKey, resource.Name, time.Duration(resource.TTLMinutes)*time.Minute, false, func() (json.RawMessage, error) {
				return resource.Fetch(ctx, client)
			})
		}
	} else if entry, ok, err := cache.ReadCachedOrEmpty(cacheKey, resource.Name); err == nil && ok {
		data = entry.Data
	}

	var metadata []fieldMetadata
	if len(data) > 0 {
		if err := json.Unmarshal(data, &metadata); err != nil {
			loadErr = errors.Join(loadErr, fmt.Errorf("decode custom field metadata: %w", err))
		}
	}
	fields, resolveErr := resolveCustomFields(entries, metadata)
	return fields, errors.Join(loadErr, resolveErr)
}

func needsFieldMetadata(entries []config.TUICustomField) bool {
	for _, entry := range entries {
		if !isCustomFieldID(strings.TrimSpace(entry.Field)) {
			return true
		}
	}
	return false
}

func resolveCustomFields(entries []config.TUICustomField, metadata []fieldMetadata) ([]core.CustomField, error) {
	byID := make(map[string]fieldMetadata, len(metadata))
	for _, f := range metadata {
		id := strings.ToLower(strings.TrimSpace(f.ID))
		if isCustomFieldID(id) {
			byID[id] = f
		}
	}

	var (
		out          []core.CustomField
		errs         []error
		seenIDs      = make(map[string]struct{}, len(entries))
		columnLabels = make(map[string]struct{}, len(entries))
	)
	for _, entry := range entries {
		selector := strings.TrimSpace(entry.Field)
		if selector == "" {
			errs = append(errs, errors.New("custom field: field is required"))
			continue
		}

		id, name, err := resolveCustomField(selector, metadata, byID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if _, exists := seenIDs[id]; exists {
			errs = append(errs, fmt.Errorf("duplicate custom field %q resolves to %s", selector, id))
			continue
		}
		seenIDs[id] = struct{}{}

		field := core.CustomField{ID: id, Name: name, Label: strings.TrimSpace(entry.Label), Column: entry.Column}
		if field.Column {
			labelKey := strings.ToLower(field.ColumnLabel())
			if _, exists := columnLabels[labelKey]; exists {
				field.Column = false
				errs = append(errs, fmt.Errorf("column label %q is already used; set a unique label for %s", field.ColumnLabel(), id))
			} else {
				columnLabels[labelKey] = struct{}{}
			}
		}
		out = append(out, field)
	}
	return out, errors.Join(errs...)
}

func resolveCustomField(selector string, metadata []fieldMetadata, byID map[string]fieldMetadata) (string, string, error) {
	id := selector
	if isCustomFieldID(id) {
		if f, ok := byID[id]; ok && strings.TrimSpace(f.Name) != "" {
			return id, strings.TrimSpace(f.Name), nil
		}
		return id, id, nil
	}

	var matches []fieldMetadata
	for _, f := range metadata {
		if isCustomFieldID(strings.ToLower(strings.TrimSpace(f.ID))) && strings.EqualFold(strings.TrimSpace(f.Name), selector) {
			matches = append(matches, f)
		}
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("custom field %q was not found", selector)
	}
	if len(matches) > 1 {
		ids := make([]string, len(matches))
		for i, f := range matches {
			ids[i] = strings.ToLower(strings.TrimSpace(f.ID))
		}
		sort.Strings(ids)
		return "", "", fmt.Errorf("custom field name %q is ambiguous: %s", selector, strings.Join(ids, ", "))
	}
	return strings.ToLower(strings.TrimSpace(matches[0].ID)), strings.TrimSpace(matches[0].Name), nil
}

func isCustomFieldID(value string) bool {
	const prefix = "customfield_"
	if !strings.HasPrefix(value, prefix) || len(value)-len(prefix) < 5 {
		return false
	}
	for _, r := range value[len(prefix):] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

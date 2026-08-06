package setting

import (
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

const (
	DefaultAutoGroupName = "auto"
	AutoGroupPrefix     = "auto"
)

var (
	autoGroupMu sync.RWMutex
	autoGroups  = map[string][]string{
		DefaultAutoGroupName: []string{"default"},
	}
)

var DefaultUseAutoGroup = false

func IsAutoGroup(group string) bool {
	group = strings.TrimSpace(group)
	return strings.HasPrefix(group, AutoGroupPrefix)
}

func ContainsAutoGroup(group string) bool {
	group = strings.TrimSpace(group)
	if !IsAutoGroup(group) {
		return false
	}
	autoGroupMu.RLock()
	defer autoGroupMu.RUnlock()
	targets, ok := autoGroups[group]
	return ok && len(targets) > 0
}

func UpdateAutoGroupsByJsonString(jsonString string) error {
	next, err := parseAutoGroupsJsonString(jsonString)
	if err != nil {
		return err
	}

	autoGroupMu.Lock()
	autoGroups = next
	autoGroupMu.Unlock()
	return nil
}

func AutoGroups2JsonString() string {
	autoGroupMu.RLock()
	legacyAutoGroups := copyStringSlice(autoGroups[DefaultAutoGroupName])
	autoGroupsCopy := copyAutoGroups(autoGroups)
	autoGroupMu.RUnlock()

	value := any(autoGroupsCopy)
	if len(autoGroupsCopy) == 0 {
		value = []string{}
	} else if len(autoGroupsCopy) == 1 {
		if _, ok := autoGroupsCopy[DefaultAutoGroupName]; ok {
			value = legacyAutoGroups
		}
	}

	jsonBytes, err := common.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func GetAutoGroups() []string {
	return GetAutoGroupsForGroup(DefaultAutoGroupName)
}

func GetAutoGroupsForGroup(group string) []string {
	group = strings.TrimSpace(group)
	if !IsAutoGroup(group) {
		return []string{}
	}

	autoGroupMu.RLock()
	defer autoGroupMu.RUnlock()
	return copyStringSlice(autoGroups[group])
}

func GetAutoGroupNames() []string {
	autoGroupMu.RLock()
	defer autoGroupMu.RUnlock()

	names := make([]string, 0, len(autoGroups))
	for name, targets := range autoGroups {
		if len(targets) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func GetAutoGroupsConfigCopy() map[string][]string {
	autoGroupMu.RLock()
	defer autoGroupMu.RUnlock()
	return copyAutoGroups(autoGroups)
}

func parseAutoGroupsJsonString(jsonString string) (map[string][]string, error) {
	trimmed := strings.TrimSpace(jsonString)
	if trimmed == "null" {
		return map[string][]string{}, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var legacyGroups []string
		if err := common.Unmarshal([]byte(jsonString), &legacyGroups); err != nil {
			return nil, err
		}
		return buildAutoGroupsFromLegacyList(legacyGroups), nil
	}
	if strings.HasPrefix(trimmed, "{") {
		var configuredGroups map[string][]string
		if err := common.Unmarshal([]byte(jsonString), &configuredGroups); err != nil {
			return nil, err
		}
		return buildAutoGroupsFromMap(configuredGroups), nil
	}

	var legacyGroups []string
	if err := common.Unmarshal([]byte(jsonString), &legacyGroups); err != nil {
		return nil, err
	}
	return buildAutoGroupsFromLegacyList(legacyGroups), nil
}

func buildAutoGroupsFromLegacyList(groups []string) map[string][]string {
	targets := normalizeAutoGroupTargets(groups)
	if len(targets) == 0 {
		return map[string][]string{}
	}
	return map[string][]string{
		DefaultAutoGroupName: targets,
	}
}

func buildAutoGroupsFromMap(configuredGroups map[string][]string) map[string][]string {
	next := make(map[string][]string, len(configuredGroups))
	for name, groups := range configuredGroups {
		name = strings.TrimSpace(name)
		if name == "" || !IsAutoGroup(name) {
			continue
		}
		targets := normalizeAutoGroupTargets(groups)
		if len(targets) == 0 {
			continue
		}
		next[name] = targets
	}
	return next
}

func normalizeAutoGroupTargets(groups []string) []string {
	targets := make([]string, 0, len(groups))
	seen := make(map[string]bool, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" || IsAutoGroup(group) || seen[group] {
			continue
		}
		seen[group] = true
		targets = append(targets, group)
	}
	return targets
}

func copyAutoGroups(source map[string][]string) map[string][]string {
	copyMap := make(map[string][]string, len(source))
	for name, groups := range source {
		copyMap[name] = copyStringSlice(groups)
	}
	return copyMap
}

func copyStringSlice(source []string) []string {
	if len(source) == 0 {
		return []string{}
	}
	copied := make([]string, len(source))
	copy(copied, source)
	return copied
}

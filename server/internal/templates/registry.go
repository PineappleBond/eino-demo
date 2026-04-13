package templates

import "sort"

// TemplateInfo is the public metadata for a template.
type TemplateInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Difficulty  string   `json:"difficulty"` // beginner, intermediate, advanced
}

// TemplateDetail includes full agent/tool config for the detail view.
type TemplateDetail struct {
	TemplateInfo
	Agents        []TemplateAgentInfo `json:"agents"`
	Tools         []TemplateToolInfo  `json:"tools"`
	InitialPrompt string              `json:"initial_prompt"`
}

// TemplateAgentInfo describes an agent in a template.
type TemplateAgentInfo struct {
	AgentKey     string `json:"agent_key"`
	AgentName    string `json:"agent_name"`
	Description  string `json:"description"`
	SystemPrompt string `json:"system_prompt"`
}

// TemplateToolInfo describes a tool in a template.
type TemplateToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Registry holds all registered templates.
var Registry = make(map[string]TemplateDetail)

// Register adds a template to the registry.
func Register(t TemplateDetail) {
	Registry[t.ID] = t
}

// List returns all template metadata sorted by ID.
func List() []TemplateInfo {
	ids := make([]string, 0, len(Registry))
	for id := range Registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]TemplateInfo, 0, len(ids))
	for _, id := range ids {
		result = append(result, Registry[id].TemplateInfo)
	}
	return result
}

// Get returns a template by ID.
func Get(id string) (TemplateDetail, bool) {
	t, ok := Registry[id]
	return t, ok
}

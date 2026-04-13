package prompts

import (
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"text/template"
)

var templateCache = make(map[string]*template.Template)
var cacheMu sync.RWMutex

// LoadTemplate loads a named prompt template by name (without .tmpl extension).
// Templates are cached after first load, safe for concurrent use.
func LoadTemplate(name string) (*template.Template, error) {
	cacheMu.RLock()
	tmpl, ok := templateCache[name]
	cacheMu.RUnlock()
	if ok {
		return tmpl, nil
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	// Double-check after acquiring write lock.
	if tmpl, ok := templateCache[name]; ok {
		return tmpl, nil
	}

	path := "templates/" + name + ".tmpl"
	content, err := fs.ReadFile(TemplatesFS, path)
	if err != nil {
		return nil, fmt.Errorf("read prompt template %q: %w", path, err)
	}

	tmpl, err = template.New(name).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse prompt template %q: %w", name, err)
	}

	templateCache[name] = tmpl
	return tmpl, nil
}

// RenderSelect loads a template with en/zh variants (separated by "---"),
// selects the correct language, and executes it.
// Returns the rendered string.
func RenderSelect(name string, data any, useChinese bool) (string, error) {
	path := "templates/" + name + ".tmpl"
	content, err := fs.ReadFile(TemplatesFS, path)
	if err != nil {
		return "", fmt.Errorf("read prompt template %q: %w", path, err)
	}

	enContent, zhContent, err := splitVariants(string(content))
	if err != nil {
		return "", fmt.Errorf("split variants for %q: %w", path, err)
	}

	selected := enContent
	if useChinese {
		selected = zhContent
	}

	tmpl, err := template.New(name).Parse(selected)
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", path, err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %q: %w", path, err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// splitVariants splits template content on "---" separator into en/zh parts.
// If no separator is found, the entire content is returned as both en and zh.
func splitVariants(content string) (en, zh string, err error) {
	content = strings.TrimSpace(content)
	// Find "---" on its own line
	idx := strings.Index(content, "\n---\n")
	if idx == -1 {
		return content, content, nil
	}
	en = strings.TrimSpace(content[:idx])
	zh = strings.TrimSpace(content[idx+len("\n---\n"):])
	if zh == "" {
		return en, en, nil
	}
	return en, zh, nil
}

// Render executes the named template with the given data and returns the rendered string.
func Render(name string, data any) (string, error) {
	tmpl, err := LoadTemplate(name)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render prompt template %q: %w", name, err)
	}
	return strings.TrimSpace(buf.String()), nil
}

package prompts

import (
	"fmt"
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

	tmpl, err := template.New(name).ParseFS(TemplatesFS, "templates/"+name+".tmpl")
	if err != nil {
		return nil, fmt.Errorf("load prompt template %q: %w", name, err)
	}

	if templateCache == nil {
		templateCache = make(map[string]*template.Template)
	}
	templateCache[name] = tmpl
	return tmpl, nil
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

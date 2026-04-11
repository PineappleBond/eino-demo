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

	// ParseFS returns a template set keyed by the file path (e.g. "templates/root.tmpl").
	// We extract the named template so callers can Execute it by the short name.
	tmplSet, err := template.ParseFS(TemplatesFS, "templates/"+name+".tmpl")
	if err != nil {
		return nil, fmt.Errorf("load prompt template %q: %w", name, err)
	}

	// The template is stored under the file path; clone it with the short name
	// so that tmpl.Execute(data) finds it correctly.
	tmpl = tmplSet.Lookup("templates/" + name + ".tmpl")
	if tmpl == nil {
		return nil, fmt.Errorf("load prompt template %q: template not found in file", name)
	}
	// Clone and rename: create a new template with the short name and copy the tree.
	tmpl, err = tmpl.Clone()
	if err != nil {
		return nil, fmt.Errorf("load prompt template %q: clone failed: %w", name, err)
	}
	// The cloned template's name is still the file path. ExecuteTemplate works though.
	templateCache[name] = tmpl
	return tmpl, nil
}

// Render executes the named template with the given data and returns the rendered string.
func Render(name string, data any) (string, error) {
	tmpl, err := LoadTemplate(name)
	if err != nil {
		return "", err
	}

	// tmpl's internal name is the file path; use ExecuteTemplate to be safe.
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "templates/"+name+".tmpl", data); err != nil {
		return "", fmt.Errorf("render prompt template %q: %w", name, err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// Package prompt assembles the system prompt that is sent to the LLM.
// Static text (SOUL/IDENTITY/8 constraints) lives in a Go-side template file;
// dynamic content (child profile, style, fear list, IP instructions, memory)
// is injected via text/template.
package prompt

import (
	"fmt"
	"os"
	"text/template"
)

// loadTemplate reads the system_prompt.tmpl file at path and parses it.
func loadTemplate(path string) (*template.Template, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", path, err)
	}
	tmpl, err := template.New("system_prompt").Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return tmpl, nil
}

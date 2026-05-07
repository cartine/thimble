package web

import (
	_ "embed"
	"html/template"
)

//go:embed template.html
var uiTemplate string

// Template parses the embedded HTML template once. The web Server holds
// the result; callers should not need to invoke this directly.
func Template() *template.Template {
	return template.Must(template.New("ui").Parse(uiTemplate))
}

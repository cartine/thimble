package web

import (
	_ "embed"
	"html/template"
)

//go:embed template.html
var uiTemplate string

// Template parses the embedded HTML template once. The file defines
// two named templates: "ui" (the authenticated dashboard) and "login"
// (the one-field token paste form). Callers should not need to invoke
// this directly; the Server constructed via New holds the result.
func Template() *template.Template {
	return template.Must(template.New("thimble").Parse(uiTemplate))
}

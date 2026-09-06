package tmpl

import "embed"

// templates embeds the FEATURE.md / WORKER.md templates into the binary
// (design.md → "Code layout → internal/tmpl/tmpl.go"; resolution #7). The
// renderer Service (render.go) substitutes data into these.
//
//go:embed feature.md.tmpl worker.md.tmpl
var templates embed.FS

// FeatureTemplate returns the embedded FEATURE.md template text.
func FeatureTemplate() (string, error) {
	b, err := templates.ReadFile("feature.md.tmpl")
	return string(b), err
}

// WorkerTemplate returns the embedded WORKER.md template text.
func WorkerTemplate() (string, error) {
	b, err := templates.ReadFile("worker.md.tmpl")
	return string(b), err
}

// RenderEmbeddedFeature renders the embedded FEATURE.md template with d.
func RenderEmbeddedFeature(d FeatureData) (string, error) {
	text, err := FeatureTemplate()
	if err != nil {
		return "", err
	}
	return RenderFeature(text, d)
}

// RenderEmbeddedWorker renders the embedded WORKER.md template with d.
func RenderEmbeddedWorker(d WorkerData) (string, error) {
	text, err := WorkerTemplate()
	if err != nil {
		return "", err
	}
	return RenderWorker(text, d)
}

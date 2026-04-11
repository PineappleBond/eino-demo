package prompts

import "embed"

// TemplatesFS embeds all prompt template files under templates/.
//
//go:embed all:templates
var TemplatesFS embed.FS

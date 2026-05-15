package static

import "embed"

//go:embed *
//go:embed assets/*/*
var FS embed.FS
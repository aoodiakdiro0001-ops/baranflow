// webui.go — الواجهة مدمجة داخل الثنائي (embed) — عرض فوري بلا أي بناء أو npm.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed ui
var files embed.FS

// UI يرجع نظام ملفات الواجهة (مجلد ui).
func UI() fs.FS {
	sub, err := fs.Sub(files, "ui")
	if err != nil {
		panic(err)
	}
	return sub
}

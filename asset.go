package alertmanager

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed ui/app/script.js ui/app/index.html ui/app/favicon.ico ui/app/lib
//go:embed template/*.tmpl template/email.html template/inline-css.js
var rawAssets embed.FS

var Assets = http.FS(PrefixFS{
	"static":    mustSub(rawAssets, "ui/app"),
	"templates": mustSub(rawAssets, "template"),
})

func mustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

type PrefixFS map[string]fs.FS

func (p PrefixFS) Open(name string) (fs.File, error) {
	for prefix, fsys := range p {
		if name == prefix {
			return fsys.Open(".")
		}

		if innerName, found := strings.CutPrefix(name, prefix+"/"); found {
			return fsys.Open(innerName)
		}
	}
	return nil, fs.ErrNotExist
}

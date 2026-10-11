// Sealed-build helper: writes the module zip for a module source tree using
// golang.org/x/mod/zip, the same packer the Go module proxy uses.
//
// Usage: go run create_mod.go <module-path> <version> <dir> <out.zip>
//
// Lives under .github, which the go tool ignores (dot-prefixed directory), so
// it never takes part in the module's own build or tests. It is built from a
// separate helper module that pins golang.org/x/mod.
package main

import (
	"fmt"
	"os"

	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: create_mod <module-path> <version> <dir> <out.zip>")
		os.Exit(2)
	}
	modPath, version, dir, out := os.Args[1], os.Args[2], os.Args[3], os.Args[4]

	f, err := os.Create(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create:", err)
		os.Exit(1)
	}
	mv := module.Version{Path: modPath, Version: version}
	if err := zip.CreateFromDir(f, mv, dir); err != nil {
		f.Close()
		fmt.Fprintln(os.Stderr, "zip.CreateFromDir:", err)
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "close:", err)
		os.Exit(1)
	}
}

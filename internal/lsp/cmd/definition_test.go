// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"context"
	"flag"
	"fmt"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/tools/go/packages/packagestest"
	"golang.org/x/tools/internal/lsp/cmd"
	"golang.org/x/tools/internal/tool"
)

var verifyGuru = flag.Bool("verify-guru", false, "Check that the guru compatability matches")

func TestDefinitionHelpExample(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Errorf("could not get wd: %v", err)
		return
	}
	thisFile := filepath.Join(dir, "definition.go")
	args := []string{"query", "definition", fmt.Sprintf("%v:#%v", thisFile, cmd.ExampleOffset)}
	// '.' added to the path class: setup-go's GOROOT contains dots (e.g. go/1.12.17/x64).
	expect := regexp.MustCompile(`^[\w/\\:_.]+flag[/\\]flag.go:\d+:\d+,\d+:\d+: defined here as type flag.FlagSet struct{.*}$`)
	got := captureStdOut(t, func() {
		tool.Main(context.Background(), &cmd.Application{}, args)
	})
	if !expect.MatchString(got) {
		t.Errorf("test with %v\nexpected:\n%s\ngot:\n%s", args, expect, got)
	}
}

func TestDefinition(t *testing.T) {
	exported := packagestest.Export(t, packagestest.GOPATH, []packagestest.Module{{
		Name:  "golang.org/fake",
		Files: packagestest.MustCopyFileTree("testdata"),
	}})
	defer exported.Cleanup()
	count := 0
	if err := exported.Expect(map[string]interface{}{
		"definition": func(fset *token.FileSet, src token.Pos, flags string, def packagestest.Range, match string) {
			count++
			args := []string{"query"}
			if flags != "" {
				args = append(args, strings.Split(flags, " ")...)
			}
			args = append(args, "definition")
			f := fset.File(src)
			loc := cmd.Location{
				Filename: f.Name(),
				Start: cmd.Position{
					Offset: f.Offset(src),
				},
			}
			loc.End = loc.Start
			args = append(args, fmt.Sprint(loc))
			app := &cmd.Application{}
			app.Config = *exported.Config
			got := captureStdOut(t, func() {
				tool.Main(context.Background(), app, args)
			})
			start := fset.Position(def.Start)
			end := fset.Position(def.End)
			expect := os.Expand(match, func(name string) string {
				switch name {
				case "file":
					return start.Filename
				case "efile":
					qfile := strconv.Quote(start.Filename)
					return qfile[1 : len(qfile)-1]
				case "line":
					return fmt.Sprint(start.Line)
				case "col":
					return fmt.Sprint(start.Column)
				case "offset":
					return fmt.Sprint(start.Offset)
				case "eline":
					return fmt.Sprint(end.Line)
				case "ecol":
					return fmt.Sprint(end.Column)
				case "eoffset":
					return fmt.Sprint(end.Offset)
				default:
					return name
				}
			})
			if *verifyGuru {
				var guruArgs []string
				runGuru := false
				for _, arg := range args {
					switch {
					case arg == "query":
						// just ignore this one
					case arg == "-json":
						guruArgs = append(guruArgs, arg)
					case arg == "-emulate=guru":
						// if we don't see this one we should not run guru
						runGuru = true
					case strings.HasPrefix(arg, "-"):
						// unknown flag, ignore it
						break
					default:
						guruArgs = append(guruArgs, arg)
					}
				}
				if runGuru {
					cmd := exec.Command("guru", guruArgs...)
					cmd.Env = exported.Config.Env
					out, err := cmd.CombinedOutput()
					if err != nil {
						t.Errorf("Could not run guru %v: %v\n%s", guruArgs, err, out)
					} else {
						guru := strings.TrimSpace(string(out))
						if !strings.HasPrefix(expect, guru) {
							t.Errorf("definition %v\nexpected:\n%s\nguru gave:\n%s", args, expect, guru)
						}
					}
				}
			}
			if expect != got {
				t.Errorf("definition %v\nexpected:\n%s\ngot:\n%s", args, expect, got)
			}
		},
	}); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatalf("No tests were run")
	}
}

// flagNames returns the command line flags tool.Main would register for a
// value of type typ, walking exported untagged struct fields recursively.
func flagNames(typ reflect.Type) []string {
	var names []string
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}
		if name, ok := field.Tag.Lookup("flag"); ok {
			names = append(names, name)
		} else if field.Type.Kind() == reflect.Struct {
			names = append(names, flagNames(field.Type)...)
		}
	}
	return names
}

// TestServeNoPortFlag checks that gopls no longer offers a -port flag, which
// started the LSP server on ":port", i.e. on all network interfaces.
func TestServeNoPortFlag(t *testing.T) {
	for _, v := range []interface{}{cmd.Serve{}, cmd.Application{}} {
		typ := reflect.TypeOf(v)
		for _, name := range flagNames(typ) {
			if name == "port" {
				t.Errorf("%v registers a -port flag, which listens on all network interfaces", typ)
			}
		}
	}
}

// TestServeListenImplicitAllInterfaces checks that -listen rejects an address
// with no host (e.g. ":0"), which would implicitly bind the LSP server to all
// network interfaces, rather than starting the server.
func TestServeListenImplicitAllInterfaces(t *testing.T) {
	s := cmd.NewServe(&cmd.Application{})
	s.Address = ":0"
	errc := make(chan error, 1)
	go func() { errc <- s.Run(context.Background()) }()
	select {
	case err := <-errc:
		if err == nil || !strings.Contains(err.Error(), "implicitly binds all network interfaces") {
			t.Errorf("Serve.Run with -listen=%s: got error %v, want rejection of implicit all-interfaces address", s.Address, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("Serve.Run with -listen=%s did not return: server is listening on all network interfaces", s.Address)
	}
}

// TestServeListenExplicitHost checks that -listen with an explicit host is
// not rejected by the all-interfaces check.
func TestServeListenExplicitHost(t *testing.T) {
	s := cmd.NewServe(&cmd.Application{})
	s.Address = "localhost:0"
	errc := make(chan error, 1)
	// The server does not return once listening; it is torn down with the
	// test binary.
	go func() { errc <- s.Run(context.Background()) }()
	select {
	case err := <-errc:
		if err != nil && strings.Contains(err.Error(), "implicitly binds all network interfaces") {
			t.Errorf("Serve.Run with -listen=%s: unexpectedly rejected: %v", s.Address, err)
		}
	case <-time.After(1 * time.Second):
		// Listening as expected.
	}
}

func captureStdOut(t testing.TB, f func()) string {
	r, out, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	defer func() {
		os.Stdout = old
		out.Close()
		r.Close()
	}()
	os.Stdout = out
	f()
	out.Close()
	data, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

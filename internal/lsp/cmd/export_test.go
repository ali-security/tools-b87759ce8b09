// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

const ExampleOffset = exampleOffset

// NewServe returns a Serve command bound to app, as Application.Run would
// set it up, so that tests can invoke Serve.Run directly.
func NewServe(app *Application) *Serve {
	return &Serve{app: app}
}

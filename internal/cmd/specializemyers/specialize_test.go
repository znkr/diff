// Copyright 2025 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSpecialize(t *testing.T) {
	got, err := specialize("../../myers/myers.go")
	if err != nil {
		t.Fatal(err)
	}

	want, err := os.ReadFile("../../myers/gen_myers_int.go")
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("differences between specialized file and checked in file detected:\n%s\nForgot to run go generate?", diff)
	}
}

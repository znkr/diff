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

// specializemyers is a bit of an abomination, it takes the myers implementation and generates a
// specialization for int. That specialization is quit a bit faster and reduces run times by -10% to
// -40%. The problem is that I haven't found a better way to do this optimization.
package main

import (
	"fmt"
	"os"
)

func main() {
	out, err := specialize("myers.go")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v", err)
		os.Exit(1)
	}

	if err := os.WriteFile("gen_myers_int.go", out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v", err)
		os.Exit(1)
	}
}

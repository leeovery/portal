// Command printversion prints the dev version of its own binary, then the
// build string the rebuild test injects, so two builds differ in content as
// two edits of a source would.
package main

import (
	"fmt"

	"github.com/leeovery/portal/internal/devversion"
)

var build string

func main() {
	fmt.Println(devversion.String())
	fmt.Println(build)
}

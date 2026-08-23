// Command abyss is the standalone, plugin-free entry point for the Abyss
// server. It builds entirely from this monorepo and is used for release
// binaries; use example/ as a template for assembling custom builds with
// plugins.
package main

import "github.com/nuln/abyss"

func main() {
	abyss.Run()
}

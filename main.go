// Command abyss is the full-featured version of the Abyss file server.
// It includes all available plugins by importing them via blank imports,
// which triggers their init() functions and registers them with the plugin framework.
package main

import (
	"github.com/nuln/abyss-core/boot"
	_ "github.com/nuln/abyss-otp"
	_ "github.com/nuln/abyss-tus"
	_ "github.com/nuln/abyss-webdav"
)

func main() {
	boot.Main()
}

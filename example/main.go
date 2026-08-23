package main

import (
	"github.com/nuln/abyss"
	_ "github.com/nuln/abyss-pro/oidc"
	_ "github.com/nuln/abyss-plugins/totp"
	_ "github.com/nuln/abyss-plugins/trash"
	_ "github.com/nuln/abyss-plugins/webdav"
)

func main() {
	abyss.Run()
}

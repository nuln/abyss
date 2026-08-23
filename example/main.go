package main

import (
	"github.com/nuln/abyss"
	_ "github.com/nuln/abyss-plugins/totp"
	_ "github.com/nuln/abyss-plugins/trash"
	_ "github.com/nuln/abyss-plugins/webdav"
	_ "github.com/nuln/abyss-pro/oidc"
)

func main() {
	abyss.Run()
}

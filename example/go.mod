module github.com/nuln/abyss-core/example

go 1.26

require (
	github.com/nuln/abyss-core v0.0.0
	github.com/nuln/abyss-pro/oidc v0.0.0-00010101000000-000000000000
	github.com/nuln/abyss-plugins/totp v0.0.0-00010101000000-000000000000
	github.com/nuln/abyss-plugins/trash v0.0.0-00010101000000-000000000000
	github.com/nuln/abyss-plugins/webdav v0.0.0-00010101000000-000000000000
)

require (
	github.com/boombuler/barcode v1.1.0 // indirect
	github.com/coreos/go-oidc/v3 v3.16.0 // indirect
	github.com/disintegration/imaging v1.6.3-0.20201218193011-d40f48ce0f09 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/pelletier/go-toml/v2 v2.3.1 // indirect
	github.com/pquerna/otp v1.5.0 // indirect
	go.etcd.io/bbolt v1.4.3 // indirect
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/image v0.40.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/text v0.37.0 // indirect
)

replace github.com/nuln/abyss-core => ../

replace github.com/nuln/abyss-pro/oidc => ../pro/oidc

replace github.com/nuln/abyss-plugins/totp => ../plugins/totp

replace github.com/nuln/abyss-plugins/webdav => ../plugins/webdav

replace github.com/nuln/abyss-plugins/trash => ../plugins/trash

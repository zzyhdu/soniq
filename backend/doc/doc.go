package doc

import _ "embed"

//go:embed openapi.yaml
var OpenAPI []byte

//go:embed api.html
var APIConsole []byte

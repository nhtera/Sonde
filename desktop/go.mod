module github.com/nhtera/sonde/desktop

go 1.26.0

toolchain go1.27.1

// npm packages may ship Go files; never build, vet or test them.
ignore ./frontend/node_modules

require (
	github.com/nhtera/sonde v0.0.0
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/wailsapp/wails/v3 v3.0.0-beta.26
)

require (
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/andybalholm/brotli v1.2.5 // indirect
	github.com/antchfx/htmlquery v1.3.6 // indirect
	github.com/antchfx/xmlquery v1.5.1 // indirect
	github.com/antchfx/xpath v1.3.8 // indirect
	github.com/bufbuild/protocompile v0.14.1 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/klauspost/compress v1.20.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/nhtera/sonde => ../

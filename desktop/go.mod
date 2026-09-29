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
	github.com/coder/websocket v1.8.15 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/nhtera/sonde => ../

# APW.exe (native Desktop shell)

`APW.exe` is the Windows desktop window for APW. It hosts the selected workspace's
`internal/desktop` server in-process through the Wails asset server, so no localhost
port is opened. It is a separate Go module so the root module stays free of Wails.

Design: `docs/superpowers/specs/2026-10-20-apw-desktop-wails-design.md`.

## Build

The Wails-dependent files carry the `desktop` build tag; plain `go test ./...` covers
everything else.

```bash
go test ./...
```

```bash
go vet -tags desktop,production ./...
```

Packaged build with icon and manifest (Wails CLI pinned to the version in `go.mod`):

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
```

```bash
wails build -s -clean -skipbindings
```

The binary lands in `build/bin/APW.exe`. `apwd.exe` must sit next to it, because
the Desktop starts the daemon for each workspace:

```bash
go build -o build/bin/apwd.exe github.com/apiwat/apw-agent/cmd/apwd
```

## Run

```bash
./build/bin/APW.exe D:\path\to\workspace
```

With no argument it reopens the last workspace, or shows the folder chooser
(`Ctrl+O`). Settings live in `%APPDATA%\APW\desktop.json`.

DevTools is compiled out of production builds. To debug, add `devtools` to the build
tags and pass `--devtools`.

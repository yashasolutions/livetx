# LiveTX Web UI — Go library stack

The web UI version of LiveTX (`cmd/livetx-web`) is built with **gsx**, a Go
HTML-templating language, plus a small set of supporting libraries. It reuses
the same `internal/app` core as the CLI and the Fyne desktop GUI — only the
presentation layer differs.

## Primary library: gsx

**Module:** `github.com/gsxhq/gsx`

gsx is the templating engine that renders the entire page server-side. You write
`.gsx` files — HTML with embedded Go control flow and typed component calls — and
the `gsx` code generator compiles each one into a `.x.go` file containing a
plain Go render function.

- **Templates:** `cmd/livetx-web/page.gsx` → generated `cmd/livetx-web/page.x.go`
- **Codegen tool:** `github.com/gsxhq/gsx/cmd/gsx` (declared as a `tool` dependency in `go.mod`)
- **Config:** `gsx.toml` wires the Tailwind class merger:
  `class_merger = "github.com/livetx/livetx/ui/merge.Merge"`

Example from `page.gsx` — HTML with Go `for`/`if` and component tags:

```go
component Page(devices []audio.Device) {
    <html lang="en" class="dark">
      ...
      <ui.NativeSelect id="device" class="min-w-56">
        { if len(devices) == 0 {
            <ui.NativeSelectOption value="">No devices found</ui.NativeSelectOption>
        } else {
            { for _, d := range devices {
                <ui.NativeSelectOption value={d.ID}>{d.Name}</ui.NativeSelectOption>
            } }
        } }
      </ui.NativeSelect>
      ...
    </html>
}
```

It renders via `Page(devices).Render(ctx, w)` inside the `/` HTTP handler
(`cmd/livetx-web/main.go`).

### gsxui component library (`ui/`)

The `ui/` package is a **gsxui** component set — server-side Go ports of
**shadcn/ui** (the shadcn v4 "new-york" registry) written as `.gsx` components
(`button.gsx`, `card.gsx`, `native-select.gsx`, `badge.gsx`, `label.gsx`, …).
Icons come from **Lucide**. See `ui/NOTICE.md` for full third-party attribution
and licenses.

## Supporting libraries

| Concern | Library | Where |
|---|---|---|
| HTML templating / components | `github.com/gsxhq/gsx` | `page.gsx`, `ui/*.gsx` |
| Tailwind class merging | `github.com/jackielii/tailwind-merge-go` | `ui/merge`, wired via `gsx.toml` |
| Live transcript transport | `github.com/gorilla/websocket` | `/ws` handler in `main.go` |
| `.env` loading | `github.com/joho/godotenv` | `main.go` startup |
| HTTP server, embedding | Go stdlib (`net/http`, `embed`, `io/fs`) | `main.go` |

Styling is compiled ahead of time: the Tailwind CLI builds `web/gsxui.css` into
`cmd/livetx-web/web/app.css`, which — along with `app.js` (the small WebSocket
client) — is embedded into the binary via `//go:embed`.

## Architecture at a glance

- **Server-side rendered.** gsx generates the full HTML page on each request; the
  browser gets static markup plus a compiled `app.css` and a small `app.js`.
- **Interactivity** (device start/stop, live transcript) rides a WebSocket
  (`gorilla/websocket`) — the browser sends `start`/`stop` commands, the server
  streams `transcript`/`status`/`error` messages back.
- **Pure Go, no cgo.** Built with `CGO_ENABLED=0`, so it cross-compiles to
  macOS/Windows/Linux from any host. On launch it opens a chromeless Chrome
  "app mode" window (falling back to the default browser) for a native feel.

## Build flow

Plain `go build` needs none of the tooling below — the generated `.x.go` files
and the compiled `app.css` are committed to the repo.

```sh
make web          # build the binary (CGO_ENABLED=0 go build ./cmd/livetx-web)
make run-web      # build and launch (Chrome app-mode window)

# Dev-only: regenerate assets after editing .gsx or Tailwind sources
make gen          # gsx generate: .gsx -> .x.go
make web-assets   # gen + Tailwind build -> cmd/livetx-web/web/app.css
```

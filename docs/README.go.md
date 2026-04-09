# CSGClaw Go Skeleton

This repository now includes a minimal Go implementation for:

- `csgclaw onboard`
- `csgclaw serve`
- `POST /api/v1/workers`

Notes:

- The BoxLite Go SDK requires Go 1.24+ with CGO enabled.
- The vendored SDK now tracks `github.com/RussellLuo/boxlite/sdks/go v0.7.6`.
- Source builds fetch `libboxlite.a` on demand via `make boxlite-setup` and the standard `make build` / `make test` targets.
- If you prefer to run the setup manually, use `(cd third_party/boxlite-go && BOXLITE_SDK_VERSION=v0.7.6 go run ./cmd/setup)`.
- The prebuilt BoxLite native library currently supports macOS arm64 and Linux amd64.

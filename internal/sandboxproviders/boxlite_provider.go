//go:build boxlite_sdk

package sandboxproviders

import (
	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	boxliteadapter "csgclaw/internal/sandbox/boxlite"
)

// The SDK-backed BoxLite provider is the only sandbox implementation behind a
// build tag because it pulls in CGO, the native BoxLite archive, and the larger
// embedded runtime payload. Other sandbox providers should remain always-on.
func init() {
	Register(config.BoxLiteSDKProvider, func(config.SandboxConfig) (agent.ServiceOption, error) {
		return agent.WithSandboxProvider(boxliteadapter.NewProvider()), nil
	})
}

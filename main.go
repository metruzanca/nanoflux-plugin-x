// Command nanoflux-plugin-x serves a private X (Twitter) plugin over gRPC so
// nanoflux can load it as an external plugin. It scrapes the profile page using
// a logged-in session (auth_token/ct0 cookies) so it can read accounts that are
// otherwise behind a login or sensitive-content wall.
//
// Build it and drop the binary into the host's plugins directory
// (NF_PLUGINS_DIR, ./plugins by default):
//
//	go -C plugins/x build -o ../nanoflux-plugin-x .
//
// See plugins/x/README.local.md (untracked) for the session config format.
package main

import (
	goplugin "github.com/hashicorp/go-plugin"

	x "nanoflux-plugin-x/x"

	"github.com/metruzanca/nanoflux/pluginapi"
)

func main() {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: pluginapi.Handshake,
		Plugins:         pluginapi.PluginSet(&x.Plugin{}),
		GRPCServer:      goplugin.DefaultGRPCServer,
	})
}

// Package version reports the Orai version: set at release build time with
// -ldflags "-X github.com/oXpace/orai/internal/version.Version=X.Y.Z", otherwise taken
// from the module build info (`go install ...@version`), otherwise "dev".
package version

import "runtime/debug"

var Version = ""

func String() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

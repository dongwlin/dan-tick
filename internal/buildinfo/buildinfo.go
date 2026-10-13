// Package buildinfo exposes metadata supplied when compiling the desktop app.
package buildinfo

// These values are set with go build -ldflags -X. Local development keeps the
// defaults instead of inferring a release version or a build time at runtime.
var (
	Version   = "dev"
	BuildTime = ""
	Commit    = ""
)

// Info is the read-only metadata snapshot exposed through Wails Binding.
type Info struct {
	Version   string `json:"version"`
	BuildTime string `json:"buildTime"`
	Commit    string `json:"commit"`
}

func Current() Info {
	return Info{Version: Version, BuildTime: BuildTime, Commit: Commit}
}

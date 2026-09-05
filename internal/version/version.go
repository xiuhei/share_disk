package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
)

// Build information set by linker flags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
	GoVersion = runtime.Version()
)

// Info contains version information.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Get returns the current version information.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: GoVersion,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// String returns a human-readable version string.
func (i Info) String() string {
	return fmt.Sprintf("share-disk %s (commit: %s, built: %s, go: %s, %s/%s)",
		i.Version, i.Commit, i.BuildTime, i.GoVersion, i.OS, i.Arch)
}

// Handler returns an HTTP handler that serves version information as JSON.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(Get()); err != nil {
			http.Error(w, `{"error":"failed to encode version"}`, http.StatusInternalServerError)
		}
	}
}

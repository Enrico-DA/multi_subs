package multicodex

import "github.com/Enrico-DA/multicodex/internal/buildinfo"

const appName = "multicodex"

func version() string {
	return buildinfo.Version
}

package multisubs

import "github.com/Enrico-DA/multi_subs/internal/buildinfo"

const appName = "multisubs"

func version() string {
	return buildinfo.Version
}

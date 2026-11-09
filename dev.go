package mygo

import "os"

// production is set to "1" by `mygo build` through
// -ldflags "-X github.com/egoist/mygo.production=1".
var production string

// IsDev reports whether the app runs in development: true unless it was
// built for production with `mygo build`, or MYGO_ENV=production is set.
// Development builds enable the web inspector by default.
func IsDev() bool {
	if production == "1" {
		return false
	}
	return os.Getenv("MYGO_ENV") != "production"
}

// devLaunched tells that `mygo dev` launched the app, which it does with
// MYGO_DEV=1: App.Relaunch then has mygo dev start it again.
var devLaunched = devMarker()

func devMarker() bool {
	set := os.Getenv("MYGO_DEV") == "1"
	// Not meant for child processes.
	os.Unsetenv("MYGO_DEV")
	return set && production != "1"
}

// devRelaunchFile is the file in which `mygo dev` looks for the arguments
// of the build that App.Relaunch has it start again, MYGO_DEV_RELAUNCH,
// which the core takes out of its environment as MYGO_DEV.
var devRelaunchFile = func() string {
	f := os.Getenv("MYGO_DEV_RELAUNCH")
	os.Unsetenv("MYGO_DEV_RELAUNCH")
	return f
}()

// launchedByDev reports whether `mygo dev` launched the app.
func launchedByDev() bool { return devLaunched }

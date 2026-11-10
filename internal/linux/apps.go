//go:build linux && (amd64 || arm64)

package linux

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo/internal/platform"
)

var (
	appsOnce sync.Once

	gAppInfoGetAll                func() ptr
	gAppInfoShouldShow            func(info ptr) bool
	gAppInfoGetDisplayName        func(info ptr) ptr
	gAppInfoLaunch                func(info, files, ctx ptr, gerr *ptr) bool
	gDesktopAppInfoGetFilename    func(info ptr) ptr
	gdkDisplayGetAppLaunchContext func(d ptr) ptr
)

func loadApps() {
	appsOnce.Do(func() {
		loadIcons() // g_desktop_app_info_new_from_filename
		mustBind(libGIO, &gAppInfoGetAll, "g_app_info_get_all")
		mustBind(libGIO, &gAppInfoShouldShow, "g_app_info_should_show")
		mustBind(libGIO, &gAppInfoGetDisplayName, "g_app_info_get_display_name")
		mustBind(libGIO, &gAppInfoLaunch, "g_app_info_launch")
		mustBind(libGIO, &gDesktopAppInfoGetFilename, "g_desktop_app_info_get_filename")
		mustBind(libGDK, &gdkDisplayGetAppLaunchContext, "gdk_display_get_app_launch_context")
	})
}

// Applications lists the applications of the desktop entries that menus
// show (g_app_info_should_show: not NoDisplay or Hidden, and meant for this
// desktop), with their names in the user's language, their desktop file
// IDs and the paths of their entries. GIO is thread-safe.
func (b *Backend) Applications() ([]platform.Application, error) {
	loadApps()
	var apps []platform.Application
	list := gAppInfoGetAll()
	for l := list; l != 0; l = field[ptr](l, 8) { // GList: data, next, prev
		info := field[ptr](l, 0)
		if gAppInfoShouldShow(info) {
			if path := goStr(gDesktopAppInfoGetFilename(info)); path != "" {
				apps = append(apps, platform.Application{
					Name: goStr(gAppInfoGetDisplayName(info)),
					ID:   goStr(gAppInfoGetID(info)),
					Path: path,
				})
			}
		}
		gObjectUnref(info)
	}
	gListFree(list)
	return apps, nil
}

// OpenApplication launches the application of a desktop entry, with the
// display's launch context, which gives it startup notification or the
// activation token that lets it take the focus.
func (shell) OpenApplication(path string) error {
	loadApps()
	info := gDesktopAppInfoNewFromFile(cs(path))
	if info == 0 {
		return fmt.Errorf("mygo: %q is not a desktop entry of an application", path)
	}
	defer gObjectUnref(info)
	ctx := gdkDisplayGetAppLaunchContext(gdkDisplayGetDefault())
	defer gObjectUnref(ctx)
	var gerr ptr
	if !gAppInfoLaunch(info, 0, ctx, &gerr) {
		return gErr(gerr)
	}
	return nil
}

// DefaultApp returns the application that opens url: the default handler
// for its scheme (g_app_info_get_default_for_uri_scheme), or for a file://
// URL, the default handler for the file's content type
// (g_content_type_guess and g_app_info_get_default_for_type).
func (shell) DefaultApp(rawURL string) (platform.Application, error) {
	loadApps()
	var scheme, ext string
	if u, e := url.Parse(rawURL); e == nil && u.Scheme != "" {
		if u.Scheme == "file" {
			ext = filepath.Ext(u.Path)
		} else {
			scheme = u.Scheme
		}
	} else {
		ext = filepath.Ext(rawURL)
	}
	var info ptr
	if scheme != "" {
		info = gAppInfoGetDefaultForURIScheme(cs(scheme))
	} else if ext != "" {
		ct := gContentTypeGuess(cs(ext), 0, 0, nil)
		if ct == 0 {
			return platform.Application{}, fmt.Errorf("mygo: no application found for %q", rawURL)
		}
		info = gAppInfoGetDefaultForType(cs(goStr(ct)), false)
		gFree(ct)
	} else {
		return platform.Application{}, fmt.Errorf("mygo: invalid URL %q", rawURL)
	}
	if info == 0 {
		return platform.Application{}, fmt.Errorf("mygo: no application found for %q", rawURL)
	}
	defer gObjectUnref(info)
	path := goStr(gDesktopAppInfoGetFilename(info))
	id := goStr(gAppInfoGetID(info))
	if path == "" {
		path = id
	}
	return platform.Application{
		Name: goStr(gAppInfoGetDisplayName(info)),
		ID:   id,
		Path: path,
	}, nil
}

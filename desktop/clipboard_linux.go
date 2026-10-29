package main

/*
#cgo pkg-config: gtk4
#include <gtk/gtk.h>
#ifdef GDK_WINDOWING_WAYLAND
#include <gdk/wayland/gdkwayland.h>
#endif

// hisar_gdk_is_wayland says whether GTK draws on Wayland, so Hisar owns the clipboard of the same display server.
static int hisar_gdk_is_wayland(void) {
#ifdef GDK_WINDOWING_WAYLAND
	GdkDisplay *display = gdk_display_get_default();
	return display != NULL && GDK_IS_WAYLAND_DISPLAY(display);
#else
	return 0;
#endif
}

// hisar_gtk_set_secret_text copies through GTK, for a compositor without data control (GNOME's): it lists
// x-kde-passwordManagerHint beside the text, which clipboard managers that check only for the type honor (Pano,
// Clipboard History, cliphist through wl-paste), but GTK never sends that type's value.
static int hisar_gtk_set_secret_text(const char *text, size_t length) {
	GdkDisplay *display = gdk_display_get_default();
	if (display == NULL) {
		return 0;
	}
	char *copy = g_strndup(text, length);
	GBytes *hint = g_bytes_new_static("secret", 6);
	GdkContentProvider *providers[2] = {
		gdk_content_provider_new_typed(G_TYPE_STRING, copy),
		gdk_content_provider_new_for_bytes("x-kde-passwordManagerHint", hint),
	};
	g_bytes_unref(hint);
	g_free(copy);
	GdkContentProvider *provider = gdk_content_provider_new_union(providers, 2);
	gboolean isSet = gdk_clipboard_set_content(gdk_display_get_clipboard(display), provider);
	g_object_unref(provider);
	return isSet;
}
*/
import "C"

import (
	"errors"
	"log"
	"sync"
	"unsafe"

	"github.com/tryhisar/hisar/desktop/secretclipboard"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// wailsClipboard marks every copy secret for clipboard managers, the Linux counterpart of the Mac's concealed
// pasteboard type: through secretclipboard on X11 and on Wayland compositors with data control (KDE, wlroots), which
// Klipper and CopyQ honor, else through GTK. It is opened at the first copy, once GTK has a display.
type wailsClipboard struct {
	app      *application.App
	mutex    sync.Mutex
	isOpened bool
	secret   *secretclipboard.Clipboard
	// isSecretCopy says whether the last copy went through secret, which then answers GetText alone, so the clear
	// never reads what another program copied since.
	isSecretCopy bool
}

// openSecret requires clipboard.mutex.
func (clipboard *wailsClipboard) openSecret() {
	clipboard.isOpened = true
	server := secretclipboard.X11
	if application.InvokeSyncWithResult(func() bool { return C.hisar_gdk_is_wayland() != 0 }) {
		server = secretclipboard.Wayland
	}
	secret, err := secretclipboard.Open(server)
	if err != nil {
		log.Printf("clipboard: copying through GTK: %v", err)
		return
	}
	clipboard.secret = secret
}

func (clipboard *wailsClipboard) SetText(text string) bool {
	clipboard.mutex.Lock()
	defer clipboard.mutex.Unlock()
	if !clipboard.isOpened {
		clipboard.openSecret()
	}
	if clipboard.secret != nil && (text != "" || clipboard.isSecretCopy) {
		err := clipboard.secret.SetText(text)
		if err == nil {
			clipboard.isSecretCopy = true
			return true
		}
		if !errors.Is(err, secretclipboard.ErrTooLarge) {
			log.Printf("clipboard: %v; copying through GTK", err)
		}
		clipboard.secret.SetText("")
	}
	clipboard.isSecretCopy = false
	// GTK is not thread-safe: write on the main thread, as Wails' own clipboard does.
	return application.InvokeSyncWithResult(func() bool {
		return C.hisar_gtk_set_secret_text((*C.char)(unsafe.Pointer(unsafe.StringData(text))), C.size_t(len(text))) != 0
	})
}

func (clipboard *wailsClipboard) GetText() (string, bool) {
	clipboard.mutex.Lock()
	secret, isSecretCopy := clipboard.secret, clipboard.isSecretCopy
	clipboard.mutex.Unlock()
	if isSecretCopy {
		return secret.GetText()
	}
	return clipboard.app.Clipboard.Text()
}

#ifndef HISAR_SECRET_CLIPBOARD_INTERNAL_H
#define HISAR_SECRET_CLIPBOARD_INTERNAL_H

#include <pthread.h>
#include <stddef.h>

#include "secretclipboard_linux.h"

// passwordManagerHint is the MIME type KDE's Klipper, CopyQ and other clipboard managers read; "secret" keeps a copy out
// of their history.
#define HISAR_PASSWORD_MANAGER_HINT "x-kde-passwordManagerHint"
#define HISAR_PASSWORD_MANAGER_HINT_VALUE "secret"

// A backend is one display server's clipboard protocol. The worker thread calls every function with the mutex held.
struct hisar_backend {
	// connection_fd is what the worker polls.
	int (*connection_fd)(hisar_secret_clipboard *clipboard);
	// prepare runs before each poll: it handles queued events and flushes requests. It returns -1 when the connection
	// is lost.
	int (*prepare)(hisar_secret_clipboard *clipboard);
	// dispatch runs after each poll; isReadable says whether the connection has data. It returns -1 when the
	// connection is lost.
	int (*dispatch)(hisar_secret_clipboard *clipboard, int isReadable);
	// claim takes the clipboard for clipboard->text; release gives it up. Both run only on the worker thread.
	int (*claim)(hisar_secret_clipboard *clipboard);
	void (*release)(hisar_secret_clipboard *clipboard);
	void (*disconnect)(hisar_secret_clipboard *clipboard);
};

struct hisar_secret_clipboard {
	const struct hisar_backend *backend;
	void *state;
	pthread_t thread;
	int wake[2];
	pthread_mutex_t mutex;
	pthread_cond_t changed;

	// The request the worker carries out next, and its result.
	int hasRequest;
	char *requestText;
	size_t requestLength;
	int result;
	int isStopping;
	// isBroken means the connection is lost: every later request fails.
	int isBroken;

	// text is what the clipboard holds while isOwned.
	char *text;
	size_t length;
	int isOwned;
};

// hisar_is_ascii says whether text may also be offered as Latin-1 STRING.
int hisar_is_ascii(const char *text, size_t length);

// hisar_drop_ownership erases the text once another program has taken the clipboard.
void hisar_drop_ownership(hisar_secret_clipboard *clipboard);

// hisar_write_all writes data to a non-blocking descriptor, giving up after a few seconds of a reader that does not
// read, and closes it.
void hisar_write_all(int fd, const char *data, size_t length);

const char *hisar_x11_connect(hisar_secret_clipboard *clipboard);
const char *hisar_wayland_connect(hisar_secret_clipboard *clipboard);

extern const struct hisar_backend hisar_x11_backend;
extern const struct hisar_backend hisar_wayland_backend;

#endif

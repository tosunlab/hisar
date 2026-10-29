#ifndef HISAR_SECRET_CLIPBOARD_H
#define HISAR_SECRET_CLIPBOARD_H

#include <stddef.h>

enum {
	HISAR_SECRET_CLIPBOARD_WAYLAND = 1,
	HISAR_SECRET_CLIPBOARD_X11 = 2,
};

enum {
	HISAR_SECRET_CLIPBOARD_SET = 0,
	HISAR_SECRET_CLIPBOARD_TOO_LARGE = 1,
	HISAR_SECRET_CLIPBOARD_FAILED = 2,
};

typedef struct hisar_secret_clipboard hisar_secret_clipboard;

// hisar_secret_clipboard_open connects to the display server on its own connection and starts the thread that serves
// the clipboard. It returns NULL, with a static sentence in *error, when the display server cannot be reached or a
// Wayland compositor has no data-control protocol.
hisar_secret_clipboard *hisar_secret_clipboard_open(int kind, const char **error);

// hisar_secret_clipboard_set takes the clipboard with a copy of text, offered as text with x-kde-passwordManagerHint
// "secret" beside it; an empty text gives up the clipboard if Hisar holds it.
int hisar_secret_clipboard_set(hisar_secret_clipboard *clipboard, const char *text, size_t length);

// hisar_secret_clipboard_get_owned returns 1 and a malloc'd copy of the text while the clipboard still holds Hisar's
// last copy, 0 once another program has taken it.
int hisar_secret_clipboard_get_owned(hisar_secret_clipboard *clipboard, char **text, size_t *length);

// hisar_secret_clipboard_close stops the thread, gives up the clipboard and erases the text.
void hisar_secret_clipboard_close(hisar_secret_clipboard *clipboard);

#endif

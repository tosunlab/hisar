#include <X11/Xatom.h>
#include <X11/Xlib.h>
#include <pthread.h>
#include <stdlib.h>
#include <string.h>

#include "internal_linux.h"

struct x11_state {
	Display *display;
	Window window;
	Atom clipboard, targets, timestamp, multiple, utf8String, text, plainUTF8, plain, hint, claimTime;
	Time ownedTime;
	size_t maxLength;
};

// Xlib has one error handler for the process. Errors on Hisar's own connections (a paste target's window that closed
// before the answer reached it) are ignored; any other goes to the handler that was there before, GTK's in the app.
static pthread_mutex_t displaysMutex = PTHREAD_MUTEX_INITIALIZER;
static Display *ownDisplays[16];
static XErrorHandler previousErrorHandler;
static int isErrorHandlerInstalled;

static int handle_error(Display *display, XErrorEvent *error) {
	pthread_mutex_lock(&displaysMutex);
	for (size_t i = 0; i < sizeof ownDisplays / sizeof ownDisplays[0]; i++) {
		if (ownDisplays[i] == display) {
			pthread_mutex_unlock(&displaysMutex);
			return 0;
		}
	}
	XErrorHandler previous = previousErrorHandler;
	pthread_mutex_unlock(&displaysMutex);
	return previous != NULL ? previous(display, error) : 0;
}

static int remember_display(Display *display) {
	pthread_mutex_lock(&displaysMutex);
	if (!isErrorHandlerInstalled) {
		previousErrorHandler = XSetErrorHandler(handle_error);
		isErrorHandlerInstalled = 1;
	}
	for (size_t i = 0; i < sizeof ownDisplays / sizeof ownDisplays[0]; i++) {
		if (ownDisplays[i] == NULL) {
			ownDisplays[i] = display;
			pthread_mutex_unlock(&displaysMutex);
			return 1;
		}
	}
	pthread_mutex_unlock(&displaysMutex);
	return 0;
}

static void forget_display(Display *display) {
	pthread_mutex_lock(&displaysMutex);
	for (size_t i = 0; i < sizeof ownDisplays / sizeof ownDisplays[0]; i++) {
		if (ownDisplays[i] == display) {
			ownDisplays[i] = NULL;
		}
	}
	pthread_mutex_unlock(&displaysMutex);
}

const char *hisar_x11_connect(hisar_secret_clipboard *clipboard) {
	struct x11_state *x11 = calloc(1, sizeof *x11);
	if (x11 == NULL) {
		return "out of memory";
	}
	x11->display = XOpenDisplay(NULL);
	if (x11->display == NULL) {
		free(x11);
		return "cannot connect to the X server";
	}
	if (!remember_display(x11->display)) {
		XCloseDisplay(x11->display);
		free(x11);
		return "too many X connections";
	}
	Display *display = x11->display;
	x11->window = XCreateSimpleWindow(display, DefaultRootWindow(display), 0, 0, 1, 1, 0, 0, 0);
	XSelectInput(display, x11->window, PropertyChangeMask);
	x11->clipboard = XInternAtom(display, "CLIPBOARD", False);
	x11->targets = XInternAtom(display, "TARGETS", False);
	x11->timestamp = XInternAtom(display, "TIMESTAMP", False);
	x11->multiple = XInternAtom(display, "MULTIPLE", False);
	x11->utf8String = XInternAtom(display, "UTF8_STRING", False);
	x11->text = XInternAtom(display, "TEXT", False);
	x11->plainUTF8 = XInternAtom(display, "text/plain;charset=utf-8", False);
	x11->plain = XInternAtom(display, "text/plain", False);
	x11->hint = XInternAtom(display, HISAR_PASSWORD_MANAGER_HINT, False);
	x11->claimTime = XInternAtom(display, "_HISAR_CLAIM_TIME", False);
	long maxRequest = XExtendedMaxRequestSize(display);
	if (maxRequest == 0) {
		maxRequest = XMaxRequestSize(display);
	}
	// A request is counted in 4-byte units; leave room for ChangeProperty's own fields.
	x11->maxLength = (size_t)maxRequest * 4 - 64;
	XFlush(display);
	clipboard->state = x11;
	return NULL;
}

static int x11_connection_fd(hisar_secret_clipboard *clipboard) {
	return ConnectionNumber(((struct x11_state *)clipboard->state)->display);
}

static void refuse(XSelectionEvent *reply) { reply->property = None; }

static void answer_request(hisar_secret_clipboard *clipboard, XSelectionRequestEvent *request) {
	struct x11_state *x11 = clipboard->state;
	Display *display = x11->display;
	XSelectionEvent reply = {
		.type = SelectionNotify,
		.display = display,
		.requestor = request->requestor,
		.selection = request->selection,
		.target = request->target,
		.property = request->property != None ? request->property : request->target,
		.time = request->time,
	};
	int isAscii = clipboard->isOwned && hisar_is_ascii(clipboard->text, clipboard->length);
	if (!clipboard->isOwned || request->selection != x11->clipboard ||
	    (request->time != CurrentTime && request->time < x11->ownedTime)) {
		refuse(&reply);
	} else if (request->target == x11->targets) {
		Atom targets[] = {x11->targets, x11->timestamp, x11->hint, x11->utf8String, x11->plainUTF8, x11->plain, x11->text, XA_STRING};
		int count = sizeof targets / sizeof targets[0] - (isAscii ? 0 : 1);
		XChangeProperty(display, request->requestor, reply.property, XA_ATOM, 32, PropModeReplace, (unsigned char *)targets, count);
	} else if (request->target == x11->timestamp) {
		long time = (long)x11->ownedTime;
		XChangeProperty(display, request->requestor, reply.property, XA_INTEGER, 32, PropModeReplace, (unsigned char *)&time, 1);
	} else if (request->target == x11->hint) {
		XChangeProperty(display, request->requestor, reply.property, x11->hint, 8, PropModeReplace,
		                (const unsigned char *)HISAR_PASSWORD_MANAGER_HINT_VALUE, strlen(HISAR_PASSWORD_MANAGER_HINT_VALUE));
	} else if (request->target == x11->utf8String || request->target == x11->plainUTF8 || request->target == x11->plain ||
	           request->target == x11->text || (request->target == XA_STRING && isAscii)) {
		Atom type = request->target == x11->text ? x11->utf8String : request->target;
		XChangeProperty(display, request->requestor, reply.property, type, 8, PropModeReplace, (const unsigned char *)clipboard->text,
		                (int)clipboard->length);
	} else {
		refuse(&reply);
	}
	XSendEvent(display, request->requestor, False, NoEventMask, (XEvent *)&reply);
}

static void x11_handle_events(hisar_secret_clipboard *clipboard) {
	struct x11_state *x11 = clipboard->state;
	while (XPending(x11->display) > 0) {
		XEvent event;
		XNextEvent(x11->display, &event);
		if (event.type == SelectionRequest && event.xselectionrequest.owner == x11->window) {
			answer_request(clipboard, &event.xselectionrequest);
		} else if (event.type == SelectionClear && event.xselectionclear.window == x11->window &&
		           event.xselectionclear.selection == x11->clipboard && event.xselectionclear.time >= x11->ownedTime) {
			hisar_drop_ownership(clipboard);
		}
	}
}

static int x11_prepare(hisar_secret_clipboard *clipboard) {
	x11_handle_events(clipboard);
	XFlush(((struct x11_state *)clipboard->state)->display);
	return 0;
}

static int x11_dispatch(hisar_secret_clipboard *clipboard, int isReadable) {
	(void)isReadable;
	x11_handle_events(clipboard);
	return 0;
}

static Bool is_claim_time(Display *display, XEvent *event, XPointer argument) {
	struct x11_state *x11 = (struct x11_state *)argument;
	(void)display;
	return event->type == PropertyNotify && event->xproperty.window == x11->window && event->xproperty.atom == x11->claimTime;
}

static int x11_claim(hisar_secret_clipboard *clipboard) {
	struct x11_state *x11 = clipboard->state;
	if (clipboard->length > x11->maxLength) {
		return HISAR_SECRET_CLIPBOARD_TOO_LARGE;
	}
	// ICCCM: own the selection with the server time of an event, which an empty property change gives.
	XChangeProperty(x11->display, x11->window, x11->claimTime, XA_STRING, 8, PropModeAppend, NULL, 0);
	XEvent event;
	XIfEvent(x11->display, &event, is_claim_time, (XPointer)x11);
	XSetSelectionOwner(x11->display, x11->clipboard, x11->window, event.xproperty.time);
	if (XGetSelectionOwner(x11->display, x11->clipboard) != x11->window) {
		return HISAR_SECRET_CLIPBOARD_FAILED;
	}
	x11->ownedTime = event.xproperty.time;
	return HISAR_SECRET_CLIPBOARD_SET;
}

static void x11_release(hisar_secret_clipboard *clipboard) {
	struct x11_state *x11 = clipboard->state;
	if (XGetSelectionOwner(x11->display, x11->clipboard) == x11->window) {
		XSetSelectionOwner(x11->display, x11->clipboard, None, x11->ownedTime);
	}
	XFlush(x11->display);
}

static void x11_disconnect(hisar_secret_clipboard *clipboard) {
	struct x11_state *x11 = clipboard->state;
	XDestroyWindow(x11->display, x11->window);
	XSync(x11->display, False);
	forget_display(x11->display);
	XCloseDisplay(x11->display);
	free(x11);
	clipboard->state = NULL;
}

const struct hisar_backend hisar_x11_backend = {
	.connection_fd = x11_connection_fd,
	.prepare = x11_prepare,
	.dispatch = x11_dispatch,
	.claim = x11_claim,
	.release = x11_release,
	.disconnect = x11_disconnect,
};

#include <errno.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <wayland-client.h>

#include "ext-data-control-v1-client-protocol.h"
#include "internal_linux.h"
#include "wlr-data-control-unstable-v1-client-protocol.h"

// The two data-control protocols, ext (staged in wayland-protocols) and wlr (wlroots, KWin), have the same requests and
// events. Hisar uses ext when the compositor has it.
enum { PROTOCOL_NONE, PROTOCOL_EXT, PROTOCOL_WLR };

struct wayland_state {
	struct wl_display *display;
	struct wl_registry *registry;
	struct wl_seat *seat;
	int protocol;
	void *manager;
	void *device;
	// source is the clipboard's current source while Hisar holds the clipboard.
	void *source;
	int isPreparedToRead;
};

static const char *textTypes[] = {"text/plain;charset=utf-8", "text/plain", "UTF8_STRING", "TEXT"};

static void handle_send(hisar_secret_clipboard *clipboard, void *source, const char *mimeType, int fd) {
	struct wayland_state *wayland = clipboard->state;
	if (!clipboard->isOwned || source != wayland->source) {
		close(fd);
	} else if (strcmp(mimeType, HISAR_PASSWORD_MANAGER_HINT) == 0) {
		hisar_write_all(fd, HISAR_PASSWORD_MANAGER_HINT_VALUE, strlen(HISAR_PASSWORD_MANAGER_HINT_VALUE));
	} else {
		hisar_write_all(fd, clipboard->text, clipboard->length);
	}
}

static void destroy_source(struct wayland_state *wayland, void *source) {
	if (wayland->protocol == PROTOCOL_EXT) {
		ext_data_control_source_v1_destroy(source);
	} else {
		zwlr_data_control_source_v1_destroy(source);
	}
}

static void handle_cancelled(hisar_secret_clipboard *clipboard, void *source) {
	struct wayland_state *wayland = clipboard->state;
	if (source == wayland->source) {
		wayland->source = NULL;
		hisar_drop_ownership(clipboard);
	}
	destroy_source(wayland, source);
}

static void ext_send(void *data, struct ext_data_control_source_v1 *source, const char *mimeType, int32_t fd) {
	handle_send(data, source, mimeType, fd);
}
static void ext_cancelled(void *data, struct ext_data_control_source_v1 *source) { handle_cancelled(data, source); }
static const struct ext_data_control_source_v1_listener extSourceListener = {.send = ext_send, .cancelled = ext_cancelled};

static void wlr_send(void *data, struct zwlr_data_control_source_v1 *source, const char *mimeType, int32_t fd) {
	handle_send(data, source, mimeType, fd);
}
static void wlr_cancelled(void *data, struct zwlr_data_control_source_v1 *source) { handle_cancelled(data, source); }
static const struct zwlr_data_control_source_v1_listener wlrSourceListener = {.send = wlr_send, .cancelled = wlr_cancelled};

// Hisar never reads the clipboard through data control, so each offer the compositor announces is destroyed when its
// selection arrives.
static void ext_data_offer(void *data, struct ext_data_control_device_v1 *device, struct ext_data_control_offer_v1 *offer) {
	(void)data, (void)device, (void)offer;
}
static void ext_selection(void *data, struct ext_data_control_device_v1 *device, struct ext_data_control_offer_v1 *offer) {
	(void)data, (void)device;
	if (offer != NULL) {
		ext_data_control_offer_v1_destroy(offer);
	}
}
static void ext_finished(void *data, struct ext_data_control_device_v1 *device) {
	struct wayland_state *wayland = ((hisar_secret_clipboard *)data)->state;
	ext_data_control_device_v1_destroy(device);
	wayland->device = NULL;
}
static const struct ext_data_control_device_v1_listener extDeviceListener = {
	.data_offer = ext_data_offer, .selection = ext_selection, .finished = ext_finished, .primary_selection = ext_selection};

static void wlr_data_offer(void *data, struct zwlr_data_control_device_v1 *device, struct zwlr_data_control_offer_v1 *offer) {
	(void)data, (void)device, (void)offer;
}
static void wlr_selection(void *data, struct zwlr_data_control_device_v1 *device, struct zwlr_data_control_offer_v1 *offer) {
	(void)data, (void)device;
	if (offer != NULL) {
		zwlr_data_control_offer_v1_destroy(offer);
	}
}
static void wlr_finished(void *data, struct zwlr_data_control_device_v1 *device) {
	struct wayland_state *wayland = ((hisar_secret_clipboard *)data)->state;
	zwlr_data_control_device_v1_destroy(device);
	wayland->device = NULL;
}
static const struct zwlr_data_control_device_v1_listener wlrDeviceListener = {
	.data_offer = wlr_data_offer, .selection = wlr_selection, .finished = wlr_finished, .primary_selection = wlr_selection};

static void handle_global(void *data, struct wl_registry *registry, uint32_t name, const char *interface, uint32_t version) {
	struct wayland_state *wayland = data;
	(void)version;
	if (strcmp(interface, ext_data_control_manager_v1_interface.name) == 0) {
		if (wayland->protocol == PROTOCOL_WLR) {
			zwlr_data_control_manager_v1_destroy(wayland->manager);
		}
		if (wayland->protocol != PROTOCOL_EXT) {
			wayland->manager = wl_registry_bind(registry, name, &ext_data_control_manager_v1_interface, 1);
			wayland->protocol = PROTOCOL_EXT;
		}
	} else if (strcmp(interface, zwlr_data_control_manager_v1_interface.name) == 0 && wayland->protocol == PROTOCOL_NONE) {
		wayland->manager = wl_registry_bind(registry, name, &zwlr_data_control_manager_v1_interface, 1);
		wayland->protocol = PROTOCOL_WLR;
	} else if (strcmp(interface, wl_seat_interface.name) == 0 && wayland->seat == NULL) {
		wayland->seat = wl_registry_bind(registry, name, &wl_seat_interface, 1);
	}
}

static void handle_global_remove(void *data, struct wl_registry *registry, uint32_t name) {
	(void)data, (void)registry, (void)name;
}

static const struct wl_registry_listener registryListener = {.global = handle_global, .global_remove = handle_global_remove};

static void wayland_disconnect(hisar_secret_clipboard *clipboard) {
	struct wayland_state *wayland = clipboard->state;
	if (wayland->source != NULL) {
		destroy_source(wayland, wayland->source);
	}
	if (wayland->protocol == PROTOCOL_EXT) {
		if (wayland->device != NULL) {
			ext_data_control_device_v1_destroy(wayland->device);
		}
		ext_data_control_manager_v1_destroy(wayland->manager);
	} else if (wayland->protocol == PROTOCOL_WLR) {
		if (wayland->device != NULL) {
			zwlr_data_control_device_v1_destroy(wayland->device);
		}
		zwlr_data_control_manager_v1_destroy(wayland->manager);
	}
	if (wayland->seat != NULL) {
		wl_seat_destroy(wayland->seat);
	}
	wl_registry_destroy(wayland->registry);
	wl_display_flush(wayland->display);
	wl_display_disconnect(wayland->display);
	free(wayland);
	clipboard->state = NULL;
}

const char *hisar_wayland_connect(hisar_secret_clipboard *clipboard) {
	struct wayland_state *wayland = calloc(1, sizeof *wayland);
	if (wayland == NULL) {
		return "out of memory";
	}
	wayland->display = wl_display_connect(NULL);
	if (wayland->display == NULL) {
		free(wayland);
		return "cannot connect to the Wayland compositor";
	}
	clipboard->state = wayland;
	wayland->registry = wl_display_get_registry(wayland->display);
	wl_registry_add_listener(wayland->registry, &registryListener, wayland);
	if (wl_display_roundtrip(wayland->display) < 0) {
		wayland_disconnect(clipboard);
		return "the Wayland compositor closed the connection";
	}
	if (wayland->protocol == PROTOCOL_NONE || wayland->seat == NULL) {
		wayland_disconnect(clipboard);
		return "the Wayland compositor has no data-control protocol";
	}
	if (wayland->protocol == PROTOCOL_EXT) {
		wayland->device = ext_data_control_manager_v1_get_data_device(wayland->manager, wayland->seat);
		ext_data_control_device_v1_add_listener(wayland->device, &extDeviceListener, clipboard);
	} else {
		wayland->device = zwlr_data_control_manager_v1_get_data_device(wayland->manager, wayland->seat);
		zwlr_data_control_device_v1_add_listener(wayland->device, &wlrDeviceListener, clipboard);
	}
	if (wl_display_roundtrip(wayland->display) < 0 || wayland->device == NULL) {
		wayland_disconnect(clipboard);
		return "the Wayland compositor refused data control";
	}
	return NULL;
}

static int wayland_connection_fd(hisar_secret_clipboard *clipboard) {
	return wl_display_get_fd(((struct wayland_state *)clipboard->state)->display);
}

static int wayland_prepare(hisar_secret_clipboard *clipboard) {
	struct wayland_state *wayland = clipboard->state;
	while (wl_display_prepare_read(wayland->display) != 0) {
		if (wl_display_dispatch_pending(wayland->display) < 0) {
			return -1;
		}
	}
	wayland->isPreparedToRead = 1;
	if (wl_display_flush(wayland->display) < 0 && errno != EAGAIN) {
		wl_display_cancel_read(wayland->display);
		wayland->isPreparedToRead = 0;
		return -1;
	}
	return 0;
}

static int wayland_dispatch(hisar_secret_clipboard *clipboard, int isReadable) {
	struct wayland_state *wayland = clipboard->state;
	if (wayland->isPreparedToRead) {
		wayland->isPreparedToRead = 0;
		if (isReadable) {
			if (wl_display_read_events(wayland->display) < 0) {
				return -1;
			}
		} else {
			wl_display_cancel_read(wayland->display);
		}
	}
	if (wl_display_dispatch_pending(wayland->display) < 0 || wayland->device == NULL) {
		return -1;
	}
	return 0;
}

static int wayland_claim(hisar_secret_clipboard *clipboard) {
	struct wayland_state *wayland = clipboard->state;
	void *source;
	if (wayland->protocol == PROTOCOL_EXT) {
		source = ext_data_control_manager_v1_create_data_source(wayland->manager);
		ext_data_control_source_v1_add_listener(source, &extSourceListener, clipboard);
		for (size_t i = 0; i < sizeof textTypes / sizeof textTypes[0]; i++) {
			ext_data_control_source_v1_offer(source, textTypes[i]);
		}
		ext_data_control_source_v1_offer(source, HISAR_PASSWORD_MANAGER_HINT);
		ext_data_control_device_v1_set_selection(wayland->device, source);
	} else {
		source = zwlr_data_control_manager_v1_create_data_source(wayland->manager);
		zwlr_data_control_source_v1_add_listener(source, &wlrSourceListener, clipboard);
		for (size_t i = 0; i < sizeof textTypes / sizeof textTypes[0]; i++) {
			zwlr_data_control_source_v1_offer(source, textTypes[i]);
		}
		zwlr_data_control_source_v1_offer(source, HISAR_PASSWORD_MANAGER_HINT);
		zwlr_data_control_device_v1_set_selection(wayland->device, source);
	}
	if (wayland->source != NULL) {
		destroy_source(wayland, wayland->source);
	}
	wayland->source = source;
	if (wl_display_flush(wayland->display) < 0 && errno != EAGAIN) {
		return HISAR_SECRET_CLIPBOARD_FAILED;
	}
	return HISAR_SECRET_CLIPBOARD_SET;
}

static void wayland_release(hisar_secret_clipboard *clipboard) {
	struct wayland_state *wayland = clipboard->state;
	if (wayland->device != NULL) {
		if (wayland->protocol == PROTOCOL_EXT) {
			ext_data_control_device_v1_set_selection(wayland->device, NULL);
		} else {
			zwlr_data_control_device_v1_set_selection(wayland->device, NULL);
		}
	}
	if (wayland->source != NULL) {
		destroy_source(wayland, wayland->source);
		wayland->source = NULL;
	}
	wl_display_flush(wayland->display);
}

const struct hisar_backend hisar_wayland_backend = {
	.connection_fd = wayland_connection_fd,
	.prepare = wayland_prepare,
	.dispatch = wayland_dispatch,
	.claim = wayland_claim,
	.release = wayland_release,
	.disconnect = wayland_disconnect,
};

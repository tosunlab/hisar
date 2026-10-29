#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <signal.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

#include "internal_linux.h"

static void erase(char **text, size_t *length) {
	if (*text != NULL) {
		explicit_bzero(*text, *length);
		free(*text);
	}
	*text = NULL;
	*length = 0;
}

int hisar_is_ascii(const char *text, size_t length) {
	for (size_t i = 0; i < length; i++) {
		if ((unsigned char)text[i] >= 0x80) {
			return 0;
		}
	}
	return 1;
}

void hisar_drop_ownership(hisar_secret_clipboard *clipboard) {
	clipboard->isOwned = 0;
	erase(&clipboard->text, &clipboard->length);
}

static long elapsed_milliseconds(const struct timespec *start) {
	struct timespec now;
	clock_gettime(CLOCK_MONOTONIC, &now);
	return (now.tv_sec - start->tv_sec) * 1000 + (now.tv_nsec - start->tv_nsec) / 1000000;
}

void hisar_write_all(int fd, const char *data, size_t length) {
	fcntl(fd, F_SETFL, fcntl(fd, F_GETFL) | O_NONBLOCK);
	struct timespec start;
	clock_gettime(CLOCK_MONOTONIC, &start);
	while (length > 0) {
		ssize_t written = write(fd, data, length);
		if (written > 0) {
			data += written;
			length -= (size_t)written;
			continue;
		}
		if (written < 0 && errno == EINTR) {
			continue;
		}
		long remaining = 3000 - elapsed_milliseconds(&start);
		if (written < 0 && errno == EAGAIN && remaining > 0) {
			struct pollfd writable = {.fd = fd, .events = POLLOUT};
			poll(&writable, 1, (int)remaining);
			continue;
		}
		break;
	}
	close(fd);
}

// carry_out_request handles the caller's text: an empty one gives up the clipboard, any other replaces Hisar's copy.
static void carry_out_request(hisar_secret_clipboard *clipboard) {
	char *text = clipboard->requestText;
	size_t length = clipboard->requestLength;
	clipboard->requestText = NULL;
	clipboard->requestLength = 0;
	if (length == 0) {
		if (clipboard->isOwned) {
			clipboard->backend->release(clipboard);
		}
		hisar_drop_ownership(clipboard);
		erase(&text, &length);
		clipboard->result = HISAR_SECRET_CLIPBOARD_SET;
		return;
	}
	char *previousText = clipboard->text;
	size_t previousLength = clipboard->length;
	int wasOwned = clipboard->isOwned;
	clipboard->text = text;
	clipboard->length = length;
	clipboard->result = clipboard->backend->claim(clipboard);
	if (clipboard->result == HISAR_SECRET_CLIPBOARD_SET) {
		clipboard->isOwned = 1;
		erase(&previousText, &previousLength);
		return;
	}
	erase(&clipboard->text, &clipboard->length);
	clipboard->text = previousText;
	clipboard->length = previousLength;
	clipboard->isOwned = wasOwned;
}

static void *run_worker(void *argument) {
	hisar_secret_clipboard *clipboard = argument;
	// A paste target that closes its pipe early must cost a failed write, not the process.
	sigset_t pipeSignal;
	sigemptyset(&pipeSignal);
	sigaddset(&pipeSignal, SIGPIPE);
	pthread_sigmask(SIG_BLOCK, &pipeSignal, NULL);

	pthread_mutex_lock(&clipboard->mutex);
	while (!clipboard->isStopping) {
		if (clipboard->hasRequest) {
			carry_out_request(clipboard);
			clipboard->hasRequest = 0;
			pthread_cond_broadcast(&clipboard->changed);
		}
		if (clipboard->backend->prepare(clipboard) < 0) {
			break;
		}
		struct pollfd descriptors[2] = {
			{.fd = clipboard->backend->connection_fd(clipboard), .events = POLLIN},
			{.fd = clipboard->wake[0], .events = POLLIN},
		};
		pthread_mutex_unlock(&clipboard->mutex);
		int ready = poll(descriptors, 2, -1);
		pthread_mutex_lock(&clipboard->mutex);
		char drained[64];
		while (read(clipboard->wake[0], drained, sizeof drained) > 0) {
		}
		int isReadable = ready > 0 && (descriptors[0].revents & POLLIN) != 0;
		if (ready > 0 && (descriptors[0].revents & (POLLERR | POLLHUP)) != 0 && !isReadable) {
			clipboard->backend->dispatch(clipboard, 0);
			break;
		}
		if (clipboard->backend->dispatch(clipboard, isReadable) < 0) {
			break;
		}
	}
	if (!clipboard->isStopping) {
		clipboard->isBroken = 1;
		hisar_drop_ownership(clipboard);
	}
	pthread_cond_broadcast(&clipboard->changed);
	pthread_mutex_unlock(&clipboard->mutex);
	return NULL;
}

hisar_secret_clipboard *hisar_secret_clipboard_open(int kind, const char **error) {
	hisar_secret_clipboard *clipboard = calloc(1, sizeof *clipboard);
	if (clipboard == NULL) {
		*error = "out of memory";
		return NULL;
	}
	if (pipe2(clipboard->wake, O_CLOEXEC | O_NONBLOCK) != 0) {
		free(clipboard);
		*error = "cannot create the clipboard thread's pipe";
		return NULL;
	}
	if (kind == HISAR_SECRET_CLIPBOARD_WAYLAND) {
		clipboard->backend = &hisar_wayland_backend;
		*error = hisar_wayland_connect(clipboard);
	} else {
		clipboard->backend = &hisar_x11_backend;
		*error = hisar_x11_connect(clipboard);
	}
	if (*error != NULL) {
		close(clipboard->wake[0]);
		close(clipboard->wake[1]);
		free(clipboard);
		return NULL;
	}
	pthread_mutex_init(&clipboard->mutex, NULL);
	pthread_cond_init(&clipboard->changed, NULL);
	if (pthread_create(&clipboard->thread, NULL, run_worker, clipboard) != 0) {
		clipboard->backend->disconnect(clipboard);
		close(clipboard->wake[0]);
		close(clipboard->wake[1]);
		free(clipboard);
		*error = "cannot start the clipboard thread";
		return NULL;
	}
	return clipboard;
}

int hisar_secret_clipboard_set(hisar_secret_clipboard *clipboard, const char *text, size_t length) {
	char *copy = malloc(length > 0 ? length : 1);
	if (copy == NULL) {
		return HISAR_SECRET_CLIPBOARD_FAILED;
	}
	memcpy(copy, text, length);
	pthread_mutex_lock(&clipboard->mutex);
	while (clipboard->hasRequest && !clipboard->isBroken) {
		pthread_cond_wait(&clipboard->changed, &clipboard->mutex);
	}
	if (clipboard->isBroken) {
		pthread_mutex_unlock(&clipboard->mutex);
		erase(&copy, &length);
		return HISAR_SECRET_CLIPBOARD_FAILED;
	}
	clipboard->requestText = copy;
	clipboard->requestLength = length;
	clipboard->hasRequest = 1;
	(void)!write(clipboard->wake[1], "x", 1);
	while (clipboard->hasRequest && !clipboard->isBroken) {
		pthread_cond_wait(&clipboard->changed, &clipboard->mutex);
	}
	int result = clipboard->result;
	if (clipboard->hasRequest) {
		erase(&clipboard->requestText, &clipboard->requestLength);
		clipboard->hasRequest = 0;
		result = HISAR_SECRET_CLIPBOARD_FAILED;
	}
	pthread_mutex_unlock(&clipboard->mutex);
	return result;
}

int hisar_secret_clipboard_get_owned(hisar_secret_clipboard *clipboard, char **text, size_t *length) {
	pthread_mutex_lock(&clipboard->mutex);
	int isOwned = clipboard->isOwned;
	if (isOwned) {
		*text = malloc(clipboard->length);
		*length = clipboard->length;
		if (*text == NULL) {
			isOwned = 0;
		} else {
			memcpy(*text, clipboard->text, clipboard->length);
		}
	}
	pthread_mutex_unlock(&clipboard->mutex);
	return isOwned;
}

void hisar_secret_clipboard_close(hisar_secret_clipboard *clipboard) {
	pthread_mutex_lock(&clipboard->mutex);
	clipboard->isStopping = 1;
	(void)!write(clipboard->wake[1], "x", 1);
	pthread_mutex_unlock(&clipboard->mutex);
	pthread_join(clipboard->thread, NULL);
	if (clipboard->isOwned) {
		clipboard->backend->release(clipboard);
	}
	hisar_drop_ownership(clipboard);
	clipboard->backend->disconnect(clipboard);
	close(clipboard->wake[0]);
	close(clipboard->wake[1]);
	pthread_mutex_destroy(&clipboard->mutex);
	pthread_cond_destroy(&clipboard->changed);
	free(clipboard);
}

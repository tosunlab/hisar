#include <stddef.h>

void *hisarPasteboardOpenGeneral(void);
void *hisarPasteboardOpenNamed(const char *name);
void hisarPasteboardReleaseNamed(void *pasteboard);
long hisarPasteboardWrite(void *pasteboard, const char *text, size_t length);
int hisarPasteboardReadOwnedString(void *pasteboard, long ownedChangeCount, char **text, size_t *length);
int hisarPasteboardHasType(void *pasteboard, const char *type);

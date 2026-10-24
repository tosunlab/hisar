#include <stdint.h>

enum {
    axfillOK = 0,
    axfillChanged = 1,
    axfillFailed = 2,
    axfillWindowNotFound = 3,
    axfillNotVisible = 4,
    axfillNoAccess = 5,
    // axfillWindowClosed: the window server no longer lists the window; window IDs are not reused.
    axfillWindowClosed = 6,
    // axfillUnreadable: the call's limit ended, a message got no answer, or a reading outgrew its budget.
    axfillUnreadable = 7,
};

// AXFillLimit limits one call: once milliseconds have passed since it began, or once *cancelled is non-zero, it sends
// no more accessibility messages and returns axfillUnreadable, and each message's timeout is what is left.
typedef struct {
    int milliseconds;
    const int32_t *cancelled;
} AXFillLimit;

// AXFillGuard names an element as a snapshot read it: every action first checks that pid still owns windowID, that the
// window element still maps to windowID, and that the element's frames still have origins, nearest first.
typedef struct {
    int pid;
    uint32_t windowID;
    uint64_t window;
    uint64_t element;
    const char *origins;
    int isSecure;
    AXFillLimit limit;
} AXFillGuard;

int axfillHasAccess(void);
void axfillRequestAccess(void);
int axfillReadWindowState(int pid, uint32_t windowID);
// axfillObserve waits up to readyMilliseconds for Chromium to build its web tree.
char *axfillObserve(int pid, uint32_t windowID, int readyMilliseconds, AXFillLimit limit, int *status);
int axfillSetValue(AXFillGuard guard, const char *value);
int axfillPress(AXFillGuard guard);
int axfillPressReturn(AXFillGuard guard);
int axfillCountCharacters(AXFillGuard guard, int *count);
char *axfillListPrompts(int pid, AXFillLimit limit, int *status);
// axfillPressPromptControl presses control of prompt if it is still a button with classes inside views with
// ancestorClasses, nearest first, each list joined by newlines.
int axfillPressPromptControl(int pid, uint64_t prompt, uint64_t control, const char *classes, const char *ancestorClasses, AXFillLimit limit);
char *axfillListWindows(int pid);

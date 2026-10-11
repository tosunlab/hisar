typedef struct {
    int pid;
    int isRegular;
    char *bundlePath;
} RunningAppInfo;

typedef struct {
    int status;
    char *teamID;
    char *signerName;
} SignatureInfo;

char *copyAppDisplayName(const char *bundlePath);
void *copyAppIconPNG(const char *bundlePath, int size, int *length);
int copyProcessWorkingDirectory(int pid, char *buffer, int size);
int copyProcessPath(int pid, char *buffer, int size);
int getResponsiblePID(int pid);
RunningAppInfo *copyAllRunningApp(int *count);
void freeAllRunningApp(RunningAppInfo *allRunningApp, int count);
SignatureInfo checkSignature(const char *path);
char *copyTeamID(const char *path);
char *copySSHRemoteHost(int pid);

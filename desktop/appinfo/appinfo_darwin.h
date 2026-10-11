char *copyAppDisplayName(const char *bundlePath);
void *copyAppIconPNG(const char *bundlePath, int size, int *length);
int copyProcessWorkingDirectory(int pid, char *buffer, int size);

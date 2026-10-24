//go:build darwin && cgo

int hisarGrantHeadSave(const char *service, const void *data, int length);
int hisarGrantHeadLoad(const char *service, void **data, int *length);
int hisarGrantHeadDelete(const char *service);
char *hisarGrantHeadDescribe(int status);

#include <stdint.h>

int hisarBiometricIsAvailable(void);
int hisarBiometricAuthenticate(const char *reason, void *window, int embed, double x, double y, double width, double height, char **errorMessage);
void hisarBiometricMovePrompt(double x, double y, double width, double height);
void hisarBiometricCancel(void);
int hisarKeychainStore(const char *service, const char *account, const void *data, int length);
int hisarKeychainLoad(const char *service, const char *account, void **data, int *length);
int hisarKeychainDelete(const char *service, const char *account);

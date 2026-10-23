#include <stdint.h>

int hisarBiometricIsAvailable(void);
int hisarBiometricAuthenticate(const char *reason, void *window, int embed, int controlSize, double x, double y, double width, double height, char **errorMessage);
void hisarBiometricMovePrompt(double x, double y, double width, double height);
void hisarBiometricCancel(void);
void hisarBiometricPrepareLoad(const char *account);
int hisarKeychainStore(const char *service, const char *account, const void *data, int length);
int hisarKeychainLoad(const char *service, const char *account, int freshContext, void **data, int *length);
int hisarKeychainLoadLegacy(const char *service, const char *account, void **data, int *length);
int hisarKeychainHasLegacy(const char *service, const char *account);
int hisarKeychainHasProtected(const char *service, const char *account);
int hisarKeychainDeleteLegacy(const char *service, const char *account);
int hisarKeychainDelete(const char *service, const char *account);

# shellcheck shell=bash disable=SC2034
# Sourced by scripts/test-ios-ui.sh and scripts/test-android-ui.sh: splits the UI test classes into CI parts of similar
# length. ui_test_part takes the classes longest first and gives each to the part with the least time so far. The last part
# runs every class the other parts do not, so a class missing from a table, such as a new one, runs there.
#
# The seconds are each class's median on GitHub's runners, measured on 2026-10-28:
# - iOS (macos-26): the "Test Suite '<class>' passed ... in N (M) seconds" lines, M, of the 18 green "iPhone UI tests" jobs of
#   macOS and iOS runs 40626545185, 40650695972, 40668395926, 40670276097, 40684250269 and 40694450582. HisarUITests,
#   AccountLifecycleUITests and PhoneApprovalUITests less the tests and steps removed after those runs, as XCTest timed them.
# - Android (ubuntu-latest, API 36 emulator): the testcase times in the JUnit XML of the android-reports-<part> artifacts of
#   runs 40292805327, 40363444054, 40375877157, 40611285397, 40611314192, 40661846716 and 40665680491, passing tests only, the
#   median per test added up per class, plus 2.114 s per test, the median gap the test orchestrator leaves between two
#   tests. ui.AccountLifecycleUITests is its two process-restart phases; a test with no passing time counts as its class's
#   median test, or every test's median, and a test only API 37 runs as the gap alone.
# Measure again when a part grows well past the others.
IOS_UI_TEST_SECONDS=(
  HisarUITests=838 OrganizationUITests=397 SharingUITests=349 AccountLifecycleUITests=295 OneTimeCodeUITests=246
  FaceIDUITests=228 FavoritesUITests=228 WideLayoutUITests=220 PasskeyUITests=171 PhoneApprovalUITests=102
)
# Classes of the package com.tryhisar.hisar.
ANDROID_UI_TEST_SECONDS=(
  autofill.AutofillServiceUITests=265 passkey.PasskeyUITests=202 ui.VaultsScreenUITests=147 AccountItemFlowUITests=135
  FileHandoffUITests=109 AutoLockUITests=92 PhoneApprovalUITests=87 ui.item.ItemEditorUITests=85 OrganizationUITests=84
  SharingUITests=84 ui.AccountLifecycleUITests=79 ui.item.ItemDetailScreenTests=71 platform.ClipboardAdapterTests=61
  ui.EditorFieldHygieneTests=55 VaultManagementUITests=51 FavoritesUITests=49 CrossDeviceSyncUITests=47
  autofill.OneTimeCodeUITests=47 PrivacyUITests=38 ui.item.ItemDetailUITests=38 ui.RootScreenUITests=37
  ui.item.PasswordGeneratorSheetUITests=37 ui.signin.QrScannerTests=37
  ui.settings.CredentialImportCancellationUITests=30 ui.CredentialImportUITests=22 ui.FieldAccessibilityTests=22
  ui.DeepLinkUITests=20 ui.signin.SignInStateTests=20 autofill.AutoFillSettingsUITests=18
  backend.HisarKitBackendTests=18 ui.item.PasswordGeneratorControlTests=18 passkey.CredentialCompletionTests=17
  ui.InvitationMenuUITests=17 ProviderScreenshotTests=14 ui.sharing.ModalSheetBarTests=10 ui.DevicesUITests=9
  ui.item.ItemEditorFieldNameTests=9 ui.UpdateRequiredScreenTests=8 platform.TrustedCertificatesTests=7
  ui.settings.PendingRecoveryStateTests=7 ui.approvals.ApprovalDetailTests=5 platform.BiometricEnrollmentTests=4
  platform.BiometricKeystoreTests=4 platform.KeystoreSecretKeyStoreTests=4 platform.StateDirectoryTests=4
  autofill.DatasetsTests=2 passkey.CredentialEntriesTests=2 ui.BiometricUITests=2 ui.item.IconPhotoConverterTests=2
  ui.settings.PasswordCharacterRuntimeTests=2
)

# ui_test_part PART COUNT CLASS=SECONDS... sets UI_TEST_PART_CLASSES. For a part below COUNT, these are the classes it runs;
# for part COUNT, they are the classes of the other parts, which it leaves out.
ui_test_part() {
  local part=$1 count=$2 class seconds best index
  local -a load=()
  shift 2
  UI_TEST_PART_CLASSES=()
  for ((index = 1; index <= count; index++)); do load[index]=0; done
  while IFS='=' read -r class seconds; do
    best=1
    for ((index = 2; index <= count; index++)); do
      if [ "${load[index]}" -lt "${load[best]}" ]; then best=$index; fi
    done
    load[best]=$((load[best] + seconds))
    if [ "$part" -lt "$count" ] && [ "$best" -eq "$part" ]; then UI_TEST_PART_CLASSES+=("$class"); fi
    if [ "$part" -eq "$count" ] && [ "$best" -ne "$count" ]; then UI_TEST_PART_CLASSES+=("$class"); fi
  done < <(printf '%s\n' "$@" | LC_ALL=C sort -t= -k2,2nr -k1,1)
}

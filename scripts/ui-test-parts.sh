# shellcheck shell=bash disable=SC2034
# Sourced by scripts/test-ios-ui.sh and scripts/test-android-ui.sh: splits the UI test classes into CI parts of similar
# length. ui_test_part takes the classes longest first and gives each to the part with the least time so far. The last part
# runs every class the other parts do not, so a class missing from a table, such as a new one, runs there.
#
# The seconds are each class's median on GitHub's runners, measured on 2026-10-23:
# - iOS (macos-26): the "Test Suite '<class>' passed ... in N (M) seconds" lines, M, of the 14 green "iPhone UI tests" jobs of
#   macOS and iOS runs 39851893409, 39871406706, 39874975576, 39899514473, 39902774426, 39905934704 and 39909260176.
# - Android (ubuntu-latest, API 36 emulator): the testcase times in the JUnit XML of 69 android-reports-<part> artifacts of
#   48 CI runs from 2026-10-22 and 2026-10-23, passing tests only, the median per test added up per class. Classes under
#   half a second, and those only API 37 runs, are left out.
# Measure again when a part grows well past the others.
IOS_UI_TEST_SECONDS=(
  HisarUITests=417 OrganizationUITests=226 FavoritesUITests=176 SharingUITests=173 AccountLifecycleUITests=143
  AutoFillUITests=132 PasskeyUITests=109 PhoneApprovalUITests=107 OneTimeCodeUITests=95 FaceIDUITests=83
)
# Classes of the package com.tryhisar.hisar.
ANDROID_UI_TEST_SECONDS=(
  PhoneApprovalUITests=121 AccountItemFlowUITests=118 autofill.AutofillServiceUITests=110 OrganizationUITests=91
  passkey.PasskeyUITests=81 ui.VaultsScreenUITests=79 SharingUITests=78 ui.item.ItemEditorUITests=78
  ui.AccountLifecycleUITests=48 platform.ClipboardAdapterTests=46 CrossDeviceSyncUITests=44 autofill.OneTimeCodeUITests=43
  ui.RootScreenUITests=43 FavoritesUITests=42 ui.EditorFieldHygieneTests=41 ui.item.ItemDetailScreenTests=35
  ui.item.ItemDetailUITests=34 ui.CredentialImportUITests=26 autofill.AutoFillUITests=21 ui.signin.QrScannerTests=20
  ui.DeepLinkUITests=16 ui.InvitationMenuUITests=16 autofill.AutoFillSettingsUITests=12 backend.HisarKitBackendTests=12
  ProviderScreenshotTests=10 ui.DevicesUITests=8 ui.signin.SignInStateTests=7 HarnessServerTests=5
  platform.TrustedCertificatesTests=3 passkey.CredentialCompletionTests=1
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

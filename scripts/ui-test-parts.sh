# shellcheck shell=bash disable=SC2034
# Sourced by scripts/test-ios-ui.sh and scripts/test-android-ui.sh: splits the UI test classes into CI parts of similar
# length. ui_test_part takes the classes longest first and gives each to the part with the least time so far. The last part
# runs every class the other parts do not, so a class missing from a table, such as a new one, runs there.
#
# The seconds are each class's time on GitHub's runners, after the UI tests were cut to the core journeys on 2026-10-28:
# - iOS (macos-26): the 2026-10-23 medians of AccountLifecycleUITests, PasskeyUITests and FaceIDUITests, which kept their
#   tests. The trimmed classes are their local run on an iPhone 17 Pro simulator (iOS 26.5) on 2026-10-28 times 0.87, the
#   median ratio of those three classes' CI medians to the same local run. PhoneApprovalUITests is its old median per test.
# - Android (ubuntu-latest, API 36 emulator): autofill.AutofillServiceUITests, PrivacyUITests and
#   platform.ClipboardAdapterTests are their tests' times in an android-reports artifact of 2026-10-28 plus the
#   orchestrator's 2.3 s between tests. The rest are estimates from a local API 37 run and the older table, with the
#   account-key steps of AccountItemFlowUITests five times slower on CI. ui.BiometricUITests and
#   BiometricKeystoreTests#testEnrollmentChangeTurnsBiometricsOff need API 37 and are skipped on CI's API 36.
# - Onboarding: iOS OnboardingUITests is its four tests' local run on 2026-10-28 times 0.87; the Android onboarding classes
#   are their local API 36 times with account-key steps five times slower and 2.3 s between tests.
# Measure again once CI has run these tables, and when a part grows well past the others.
IOS_UI_TEST_SECONDS=(
  HisarUITests=173 AccountLifecycleUITests=133 SharingUITests=124 PasskeyUITests=109 OrganizationUITests=100
  FaceIDUITests=83 PhoneApprovalUITests=17 OnboardingUITests=414
)
# Classes of the package com.tryhisar.hisar.
ANDROID_UI_TEST_SECONDS=(
  passkey.PasskeyUITests=110 autofill.AutofillServiceUITests=74 AccountItemFlowUITests=70 FileHandoffUITests=61
  PhoneApprovalUITests=60 PrivacyUITests=55 ui.AccountLifecycleUITests=50 SharingUITests=45 OrganizationUITests=30
  backend.HisarKitBackendTests=18 platform.ClipboardAdapterTests=16 platform.KeystoreSecretKeyStoreTests=9
  platform.BiometricKeystoreTests=7 ui.BiometricUITests=3 OnboardingUITests=28 OnboardingServerUITests=45
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

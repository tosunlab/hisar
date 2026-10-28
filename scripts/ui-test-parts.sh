# shellcheck shell=bash disable=SC2034
# Sourced by scripts/test-ios-ui.sh and scripts/test-android-ui.sh: splits the UI test classes into CI parts of similar
# length. ui_test_part takes the classes longest first and gives each to the part with the least time so far. The last part
# runs every class the other parts do not, so a class missing from a table, such as a new one, runs there.
#
# The seconds are each class's estimated time on GitHub's runners after the UI tests were cut to the core journeys on
# 2026-10-28 (ESTIMATES, replaced below once measured).
IOS_UI_TEST_SECONDS=(
  AccountLifecycleUITests=133 SharingUITests=120 OrganizationUITests=110 PasskeyUITests=109 HisarUITests=165
  FaceIDUITests=83 PhoneApprovalUITests=17
)
# Classes of the package com.tryhisar.hisar.
ANDROID_UI_TEST_SECONDS=(
  AccountItemFlowUITests=90 passkey.PasskeyUITests=80 autofill.AutofillServiceUITests=74 SharingUITests=60
  PhoneApprovalUITests=60 FileHandoffUITests=56 PrivacyUITests=55 OrganizationUITests=50 ui.AccountLifecycleUITests=50
  ui.BiometricUITests=40 backend.HisarKitBackendTests=18 platform.ClipboardAdapterTests=16
  platform.KeystoreSecretKeyStoreTests=9 platform.BiometricKeystoreTests=5
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

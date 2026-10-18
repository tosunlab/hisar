package com.tryhisar.hisar.model

object OrganizationLogic {
    const val MAX_NAME_LENGTH = 100

    /** Organization and team names are 1 to 100 characters, as apitypes.CleanOrganizationName checks again. */
    fun isValidName(name: String): Boolean {
        val trimmed = name.trimmingWhitespacesAndNewlines()
        return trimmed.isNotEmpty() && trimmed.codePointCount(0, trimmed.length) <= MAX_NAME_LENGTH
    }
}

package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.UpdateRequirement

/** The one screen the app shows, from the backend's status; overlays such as notices sit above any of them. */
sealed interface Screen {
    data object Starting : Screen

    data class Failed(val message: String) : Screen

    data class UpdateRequired(val requirement: UpdateRequirement) : Screen

    data object SignedOut : Screen

    data object Locked : Screen

    data object Unlocked : Screen
}

/** The unlocked app's tabs, in the order the tab bar shows them. */
enum class Tab {
    VAULTS,
    APPROVALS,
    SETTINGS,
    SEARCH,
}

/** The screens the Settings tab pushes, the iPhone app's SettingsDestination. */
enum class SettingsDestination {
    ACCOUNT,
    SECRET_KEY,
    SECURITY,
    APPEARANCE,
    MAC_APPROVALS,
    NOTIFICATIONS,
    AUTOFILL,
    PASSWORD_GENERATOR,
    PEOPLE,
    ORGANIZATIONS,
    DEVICES,
    HELP,
    ABOUT,
    CHANGE_MASTER_PASSWORD,
    REPLACE_SECRET_KEY,
    DELETE_ACCOUNT,
    EXPORT_DATA,
}

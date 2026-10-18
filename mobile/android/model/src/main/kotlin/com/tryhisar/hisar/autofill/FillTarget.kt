package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.model.LoginMatch
import com.tryhisar.hisar.model.VaultLogic
import kotlinx.serialization.Serializable

/**
 * An Android app as Hisar links logins to it: its package name and the SHA-256 of its signing certificate in hex. The
 * package alone is not enough, since any sideloaded app can take a package name.
 */
data class AndroidApp(val packageName: String, val certificateSHA256: String)

/** What AutoFill fills into: pages of these websites, or an Android app with the name the user knows it by. */
sealed interface FillTarget {
    data class Websites(val allServiceIdentifier: List<String>) : FillTarget

    data class App(val app: AndroidApp, val label: String) : FillTarget

    /** The site or app a request names on screen: the first host, or the app's name. */
    val name: String
        get() = when (this) {
            is Websites -> allServiceIdentifier.firstNotNullOfOrNull(VaultLogic::extractHost) ?: allServiceIdentifier.firstOrNull().orEmpty()
            is App -> label
        }
}

/** Why a login matches a request, which is also how Hisar checks it again before it hands out the secret. */
sealed interface LoginSource {
    /** The login is saved for this website, or for a website whose Digital Asset Links name the app. */
    data class Website(val serviceIdentifier: String) : LoginSource

    /** The user linked the login to this app ("Always use for this app"). */
    data class App(val app: AndroidApp) : LoginSource
}

enum class RequestKind { PASSWORD, ONE_TIME_CODE }

/** A login the user picked from a suggestion, filled once Hisar is unlocked. */
data class PendingCredentialRequest(val recordIdentifier: String, val source: LoginSource, val kind: RequestKind)

/** A login that matches a request, with how it matched. */
data class MatchedLogin(val login: LoginMatch, val source: LoginSource)

/** The host of a login's website and its registrable domain, as core/app lists them for Digital Asset Links. */
@Serializable
data class LoginWebsite(val host: String, val registrableDomain: String = "")

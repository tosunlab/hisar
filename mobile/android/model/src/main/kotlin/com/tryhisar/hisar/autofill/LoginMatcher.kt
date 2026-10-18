package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.model.LoginCredential
import com.tryhisar.hisar.model.OneTimeCode
import com.tryhisar.hisar.model.VaultBackend

/** Answers which of the vault's websites vouch for an app in their Digital Asset Links, as service identifiers. */
fun interface AppWebsiteVerifier {
    /** Blocks on the network; a site that cannot be asked does not vouch. */
    fun listAllVouchingWebsite(app: AndroidApp, allWebsite: List<LoginWebsite>): List<String>
}

/**
 * The logins a fill request may use and their secrets, shared by the AutoFill service, which offers the suggestions, and
 * the AutoFill screens, which fill them. Every call blocks on the backend.
 */
class LoginMatcher(private val backend: VaultBackend, private val verifier: AppWebsiteVerifier) {
    /** Each matching login once, with the first way it matched: an app's own links come before its websites. */
    fun listAllMatch(target: FillTarget): List<MatchedLogin> {
        val allMatch = when (target) {
            is FillTarget.Websites -> listAllWebsiteMatch(target.allServiceIdentifier)
            is FillTarget.App -> {
                val linked = backend.listAllLoginForAndroidApp(target.app.packageName, target.app.certificateSHA256)
                    .map { MatchedLogin(it, LoginSource.App(target.app)) }
                val allWebsite = backend.listAllLoginWebsite()
                linked + listAllWebsiteMatch(if (allWebsite.isEmpty()) emptyList() else verifier.listAllVouchingWebsite(target.app, allWebsite))
            }
        }
        return allMatch.distinctBy { it.login.id }
    }

    private fun listAllWebsiteMatch(allServiceIdentifier: List<String>): List<MatchedLogin> = allServiceIdentifier.flatMap { serviceIdentifier ->
        backend.listAllLoginForService(serviceIdentifier).map { MatchedLogin(it, LoginSource.Website(serviceIdentifier)) }
    }

    /** The login's username and password after the backend checks again that it matches; without a source, any login. */
    fun getLogin(vaultID: String, itemID: String, source: LoginSource?): LoginCredential = when (source) {
        is LoginSource.Website -> backend.getLoginForService(vaultID, itemID, source.serviceIdentifier)
        is LoginSource.App -> backend.getLoginForAndroidApp(vaultID, itemID, source.app.packageName, source.app.certificateSHA256)
        null -> backend.getItem(vaultID, itemID).let { LoginCredential(it.username, it.password, it.hasOneTimePassword) }
    }

    fun getOneTimeCode(vaultID: String, itemID: String, source: LoginSource?): OneTimeCode = when (source) {
        is LoginSource.Website -> backend.getOneTimeCodeForService(vaultID, itemID, source.serviceIdentifier)
        is LoginSource.App -> backend.getOneTimeCodeForAndroidApp(vaultID, itemID, source.app.packageName, source.app.certificateSHA256)
        null -> backend.getOneTimeCode(vaultID, itemID)
    }
}

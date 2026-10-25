package com.tryhisar.hisar.autofill

import android.app.PendingIntent
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.os.CancellationSignal
import android.service.autofill.AutofillService
import android.service.autofill.FillCallback
import android.service.autofill.FillRequest
import android.service.autofill.FillResponse
import android.service.autofill.SaveCallback
import android.service.autofill.SaveInfo
import android.service.autofill.SaveRequest
import android.util.Log
import android.view.autofill.AutofillId
import androidx.core.os.BundleCompat
import com.tryhisar.hisar.BackendProvider
import com.tryhisar.hisar.MainActivity
import com.tryhisar.hisar.model.LoginMatch
import com.tryhisar.hisar.model.VaultStatus
import java.util.concurrent.Executors

/**
 * Hisar's AutoFill service, in the app's process on the app's one backend (D2). Locked, it offers only "Unlock Hisar"
 * (D3); unlocked, one suggestion per matching login and "Search Hisar". After a sign-in it offers to save what the user
 * typed, which opens the app's editor (D15).
 */
class HisarAutofillService : AutofillService() {
    private val executor = Executors.newSingleThreadExecutor()

    override fun onFillRequest(request: FillRequest, cancellationSignal: CancellationSignal, callback: FillCallback) {
        val fillContext = request.fillContexts.last()
        val structure = fillContext.structure
        val packageName = structure.activityComponent.packageName
        if (packageName == this.packageName) {
            callback.onSuccess(null)
            return
        }
        val allRoot = structure.readFormNodes(fillContext.focusedId)
        val task = executor.submit {
            val response = try {
                val app = InstalledApps.readAndroidApp(this, packageName)
                FillRequestParser.parse(allRoot, packageName, app?.certificateSHA256, PrivilegedBrowserList.read(this))?.let { form ->
                    val inlineRequest = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) request.inlineSuggestionsRequest else null
                    buildResponse(form, app, Datasets(this, inlineRequest, request.id))
                }
            } catch (error: Exception) {
                Log.w("Hisar", "AutoFill could not answer", error)
                null
            }
            if (!cancellationSignal.isCanceled) callback.onSuccess(response)
        }
        cancellationSignal.setOnCancelListener { task.cancel(true) }
    }

    private fun buildResponse(form: LoginForm<AutofillId>, app: AndroidApp?, datasets: Datasets): FillResponse {
        val target = when (val source = form.source) {
            is AutofillSource.Web -> FillTarget.Websites(listOf(source.serviceIdentifier))
            is AutofillSource.UnknownPage -> FillTarget.Websites(emptyList())
            is AutofillSource.App -> app?.let { FillTarget.App(it, InstalledApps.readLabel(this, source.packageName)) } ?: FillTarget.Websites(emptyList())
        }
        val request = AutofillRequest(form.username, form.password, form.oneTimeCode, form.kind, target)
        val response = FillResponse.Builder()
        for ((index, suggestion) in listAllSuggestion(request, datasets).withIndex()) {
            response.addDataset(datasets.buildSuggestion(suggestion.first, suggestion.second, index))
        }
        buildSaveInfo(form, target)?.let { (saveInfo, clientState) ->
            response.setSaveInfo(saveInfo)
            response.setClientState(clientState)
        }
        return response.build()
    }

    private fun listAllSuggestion(request: AutofillRequest, datasets: Datasets): List<Pair<AutofillRequest, Suggestion>> {
        val backend = (application as BackendProvider).backend
        val status = backend.getStatus()
        val name = request.target.name
        val noun = if (request.kind == RequestKind.ONE_TIME_CODE) "a one-time code" else "a login"
        if (!status.isSignedIn) return listOf(request to Suggestion("Sign in to Hisar", "Open Hisar to sign in"))
        if (status.state != VaultStatus.STATE_UNLOCKED || status.updateRequired != null) {
            return listOf(request to Suggestion("Unlock Hisar", if (name.isEmpty()) "" else "Fill $noun for $name"))
        }
        val allMatch = LoginMatcher(backend, HttpAssetLinksVerifier).listAllMatch(request.target)
            .filter { request.kind == RequestKind.PASSWORD || it.login.hasOneTimePassword }
        val limit = (datasets.inlineLimit ?: MAXIMUM_SUGGESTIONS).coerceAtMost(MAXIMUM_SUGGESTIONS) - 1
        val allLogin = allMatch.take(limit).map { match ->
            request.copy(pick = AutofillRequest.Pick(match.login.id, match.source)) to Suggestion(match.login.title, describe(match.login))
        }
        return allLogin + (request to Suggestion("Search Hisar", "Search all logins"))
    }

    private fun describe(login: LoginMatch): String = login.username.ifEmpty { login.vaultName }

    private fun buildSaveInfo(form: LoginForm<AutofillId>, target: FillTarget): Pair<SaveInfo, Bundle>? {
        val password = form.password ?: return null
        val clientState = Bundle().apply {
            putParcelable(STATE_USERNAME, form.username)
            putParcelable(STATE_PASSWORD, password)
            when (target) {
                is FillTarget.Websites -> putString(STATE_DOMAIN, (form.source as? AutofillSource.Web)?.domain ?: return null)
                is FillTarget.App -> {
                    putString(STATE_APP_PACKAGE, target.app.packageName)
                    putString(STATE_APP_CERTIFICATE, target.app.certificateSHA256)
                    putString(STATE_APP_LABEL, target.label)
                }
            }
        }
        val saveInfo = SaveInfo.Builder(SaveInfo.SAVE_DATA_TYPE_USERNAME or SaveInfo.SAVE_DATA_TYPE_PASSWORD, arrayOf(password))
            .apply { form.username?.let { setOptionalIds(arrayOf(it)) } }
            .setFlags(SaveInfo.FLAG_SAVE_ON_ALL_VIEWS_INVISIBLE)
            .build()
        return saveInfo to clientState
    }

    override fun onSaveRequest(request: SaveRequest, callback: SaveCallback) {
        val clientState = request.clientState
        val passwordID = clientState?.let { BundleCompat.getParcelable(it, STATE_PASSWORD, AutofillId::class.java) }
        val usernameID = clientState?.let { BundleCompat.getParcelable(it, STATE_USERNAME, AutofillId::class.java) }
        val allStructure = request.fillContexts.reversed().map { it.structure }
        fun readText(id: AutofillId?): String = id?.let { allStructure.firstNotNullOfOrNull { structure -> structure.readText(id) } }.orEmpty()
        val password = readText(passwordID)
        val place = clientState?.let(::readPlace)
        if (password.isEmpty() || place == null) {
            callback.onSuccess()
            return
        }
        SavedLoginInbox.put(SavedLogin(readText(usernameID), password, place))
        val intent = Intent(this, MainActivity::class.java).setAction(SavedLoginInbox.ACTION_SAVE_LOGIN)
        callback.onSuccess(PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT).intentSender)
    }

    private fun readPlace(clientState: Bundle): SavedLogin.Place? {
        clientState.getString(STATE_DOMAIN)?.let { return SavedLogin.Place.Website(it) }
        val packageName = clientState.getString(STATE_APP_PACKAGE) ?: return null
        val certificate = clientState.getString(STATE_APP_CERTIFICATE) ?: return null
        return SavedLogin.Place.App(AndroidApp(packageName, certificate), clientState.getString(STATE_APP_LABEL) ?: packageName)
    }

    override fun onDestroy() {
        executor.shutdown()
        super.onDestroy()
    }

    private companion object {
        const val MAXIMUM_SUGGESTIONS = 10
        const val STATE_USERNAME = "username"
        const val STATE_PASSWORD = "password"
        const val STATE_DOMAIN = "domain"
        const val STATE_APP_PACKAGE = "appPackage"
        const val STATE_APP_CERTIFICATE = "appCertificate"
        const val STATE_APP_LABEL = "appLabel"
    }
}

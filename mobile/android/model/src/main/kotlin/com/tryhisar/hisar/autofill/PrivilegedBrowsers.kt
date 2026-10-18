package com.tryhisar.hisar.autofill

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/**
 * The apps whose reported page addresses AutoFill trusts, each pinned to the SHA-256 of a certificate it is signed with,
 * so an app that takes a browser's package name is still that app. [allBrowser] comes from `privileged_browsers.json`, the
 * list the passkey provider hands to Credential Manager. [allWebViewApp] name pages like a browser but are apps otherwise:
 * only the debug build's instrumented tests.
 */
class PrivilegedBrowsers(private val allBrowser: Set<AndroidApp>, private val allWebViewApp: Set<AndroidApp> = emptySet()) {
    fun isBrowser(app: AndroidApp?): Boolean = app in allBrowser

    fun isNamingPages(app: AndroidApp?): Boolean = app in allBrowser || app in allWebViewApp

    @Serializable
    private class AppList(val apps: List<Entry>)

    @Serializable
    private class Entry(val type: String, val info: Info)

    @Serializable
    private class Info(@SerialName("package_name") val packageName: String, val signatures: List<Signature>)

    @Serializable
    private class Signature(@SerialName("cert_fingerprint_sha256") val certificateSHA256: String)

    companion object {
        private val json = Json { ignoreUnknownKeys = true }

        /** Reads Credential Manager's privileged-apps format, whose fingerprints are colon-separated hex as [AndroidApp]'s. */
        fun parse(text: String, allWebViewApp: Set<AndroidApp> = emptySet()): PrivilegedBrowsers {
            val allBrowser = json.decodeFromString<AppList>(text).apps.filter { it.type == "android" }.flatMapTo(HashSet()) { entry ->
                entry.info.signatures.map { AndroidApp(entry.info.packageName, it.certificateSHA256.uppercase()) }
            }
            return PrivilegedBrowsers(allBrowser, allWebViewApp)
        }
    }
}

package com.tryhisar.hisar.app

import java.net.URI
import java.net.URISyntaxException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.io.bytestring.ByteString

/**
 * The icons of logins' websites, asked once per domain during an unlock and forgotten on lock and when the setting
 * changes. The backend fetches each from the website itself, never from a third party.
 */
class WebsiteIconModel(private val context: ModelContext) {
    private val mutableIconByDomain = MutableStateFlow<Map<String, ByteString>>(emptyMap())

    /** The domains asked during this unlock, finished or not, so a website without an icon is asked once. */
    private val jobByDomain = mutableMapOf<String, Job>()

    /** The icons loaded during this unlock by domain; a domain still loading, or whose website has none, is missing. */
    val iconByDomain: StateFlow<Map<String, ByteString>> = mutableIconByDomain.asStateFlow()

    init {
        context.addStatusListener { previous, current ->
            if (previous.isUnlocked && !current.isUnlocked) forgetWebsiteIcons()
        }
    }

    fun loadWebsiteIcon(url: String) {
        if (!context.isWebsiteIconsEnabled.value) return
        val domain = getWebsiteIconDomain(url) ?: return
        if (domain in mutableIconByDomain.value || domain in jobByDomain) return
        jobByDomain[domain] = context.scope.launch {
            val icon = context.attempt { it.getWebsiteIcon(url) } ?: return@launch
            mutableIconByDomain.update { it + (domain to icon) }
        }
    }

    fun getWebsiteIcon(url: String): ByteString? = getWebsiteIconDomain(url)?.let { mutableIconByDomain.value[it] }

    suspend fun setWebsiteIconsEnabled(isEnabled: Boolean) {
        context.perform(isBusyShown = false) { it.setWebsiteIconsEnabled(isEnabled) } ?: return
        context.isWebsiteIconsEnabled.value = isEnabled
        forgetWebsiteIcons()
    }

    private fun forgetWebsiteIcons() {
        jobByDomain.values.forEach { it.cancel() }
        jobByDomain.clear()
        mutableIconByDomain.value = emptyMap()
    }

    companion object {
        /** The lower-case host of an https URL, which icons are kept by; null for any other URL. */
        fun getWebsiteIconDomain(url: String): String? {
            val uri = try {
                URI(url.trim())
            } catch (_: URISyntaxException) {
                return null
            }
            if (uri.scheme?.lowercase() != "https") return null
            return uri.host?.lowercase()?.takeIf { it.isNotEmpty() }
        }
    }
}

package com.tryhisar.hisar.app

/** The device clipboard for text the app copies itself rather than through Go. */
interface TextClipboard {
    /** Copies a secret, such as a Secret Key or a website, as Go copies fields: marked sensitive and cleared after about 90 seconds. */
    fun copySecret(text: String): Boolean

    /** Copies text that is not secret, such as the support address or diagnostics: never cleared. */
    fun copyPlain(text: String): Boolean
}

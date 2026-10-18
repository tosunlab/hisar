package com.tryhisar.hisar.debug

import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.fragment.app.FragmentActivity

/**
 * Debug builds only: hosts a screen the system or another device normally opens, with a made-up request named by the
 * intent's extras, for UI tests (plan §4.13). Each preview reads its own extras.
 */
class PreviewActivity : FragmentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        val content = AutoFillPreview.read(this, intent) ?: return finish()
        setContent { content() }
    }
}

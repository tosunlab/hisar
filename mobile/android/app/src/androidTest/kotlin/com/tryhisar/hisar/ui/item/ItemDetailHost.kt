package com.tryhisar.hisar.ui.item

import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.test.core.app.ActivityScenario
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.app.TextClipboard
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.platform.waitUntil
import com.tryhisar.hisar.ui.components.NoticeHost
import com.tryhisar.hisar.ui.components.exposeTestTags
import com.tryhisar.hisar.ui.theme.HisarTheme
import java.io.Closeable
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking

/**
 * The item detail screen on [backend] in an activity of its own with focus, as the Vaults tab pushes it, with the app's
 * notice over it. The Vaults tab is not part of these tests: [show] opens an item as tapping its row would.
 */
class ItemDetailHost(private val backend: VaultBackend, clipboard: TextClipboard) : Closeable {
    private val scope = MainScope()
    val model = AppModel(scope, Dispatchers.IO, clipboard)
    val allEditorRequest = mutableListOf<EditorRequest>()
    var backCount = 0
        private set
    private var shownItem by mutableStateOf<Pair<String, String>?>(null)
    private val scenario: ActivityScenario<ComponentActivity>

    init {
        runBlocking(Dispatchers.Main) {
            model.start { backend }
            model.refreshAll()
        }
        scenario = ActivityScenario.launch(ComponentActivity::class.java)
        scenario.onActivity { activity ->
            activity.enableEdgeToEdge()
            activity.setContent {
                HisarTheme {
                    Scaffold(
                        snackbarHost = { NoticeHost(model) },
                        modifier = Modifier.exposeTestTags(),
                    ) { padding ->
                        androidx.compose.foundation.layout.Box(Modifier.fillMaxSize().padding(padding)) {
                            shownItem?.let { (vaultID, itemID) ->
                                key(vaultID, itemID) {
                                    ItemDetailScreen(model, vaultID, itemID, onEdit = { allEditorRequest += it }, onBack = { backCount += 1 })
                                }
                            }
                        }
                    }
                }
            }
        }
        waitUntil(10_000, "the activity has window focus") {
            var isFocused = false
            scenario.onActivity { isFocused = it.hasWindowFocus() }
            isFocused
        }
    }

    fun show(vaultID: String, itemID: String) {
        scenario.onActivity { shownItem = vaultID to itemID }
    }

    /** Reads the vaults and items again, as the app does after a change elsewhere. */
    fun refresh() = runBlocking(Dispatchers.Main) { model.refreshAll() }

    override fun close() {
        scenario.close()
        scope.cancel()
    }
}

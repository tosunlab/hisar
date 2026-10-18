package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FakeBackend
import java.time.Instant
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope

/**
 * An app model on a [FakeBackend] for a test in `runTest`: its main work and its backend calls run on the test's
 * scheduler, so `runCurrent()` finishes what the model started, and its clock reads [currentDate], which the test moves.
 */
class AppModelHarness(private val testScope: TestScope, val backend: FakeBackend = FakeBackend()) {
    var currentDate: Instant = Instant.ofEpochSecond(1_800_000_000)
    val events = MutableSharedFlow<BackendEvent>()
    val clipboard = FakeTextClipboard()

    fun createModel(ioDispatcher: CoroutineDispatcher = StandardTestDispatcher(testScope.testScheduler)): AppModel =
        AppModel(testScope.backgroundScope, ioDispatcher, clipboard, events, now = { currentDate })

    suspend fun startModel(): AppModel = createModel().also { model -> model.start { backend } }

    suspend fun startUnlockedModel(): AppModel {
        backend.seedSignedIn()
        return startModel().also { it.refreshAll() }
    }
}

/** A clipboard that keeps what the app copied, secret and plain apart. */
class FakeTextClipboard : TextClipboard {
    val allCopiedSecret = mutableListOf<String>()
    val allCopiedPlain = mutableListOf<String>()

    override fun copySecret(text: String): Boolean = allCopiedSecret.add(text)

    override fun copyPlain(text: String): Boolean = allCopiedPlain.add(text)
}

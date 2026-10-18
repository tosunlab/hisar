package com.tryhisar.hisar

import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.tryhisar.hisar.app.AppModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

/**
 * Keeps the app model across configuration changes, starts it on the Application's backend, and tells it when the whole
 * app goes to the background and comes back, which locks it after the auto-lock delay.
 */
class AppViewModel(private val provider: BackendProvider) : ViewModel() {
    val model = AppModel(viewModelScope, Dispatchers.IO, provider.events)
    private var hasEnteredBackground = false

    private val processObserver = object : DefaultLifecycleObserver {
        override fun onStop(owner: LifecycleOwner) {
            hasEnteredBackground = true
            model.handleDidEnterBackground()
        }

        override fun onStart(owner: LifecycleOwner) {
            if (hasEnteredBackground) {
                hasEnteredBackground = false
                viewModelScope.launch {
                    model.handleWillEnterForeground()
                    model.reportActivity()
                }
            } else {
                model.reportActivity()
            }
        }
    }

    init {
        viewModelScope.launch { model.start { provider.backend } }
        ProcessLifecycleOwner.get().lifecycle.addObserver(processObserver)
    }

    fun copySecret(text: String): Boolean = provider.copySecret(text)

    override fun onCleared() {
        ProcessLifecycleOwner.get().lifecycle.removeObserver(processObserver)
    }
}

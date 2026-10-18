package com.tryhisar.hisar

import android.os.Bundle
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.backendJson
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisarkit.Hisarkit
import com.tryhisar.hisarkit.SecretKeyStore
import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        setContent {
            HisarTheme {
                val description by produceState<String?>(null) {
                    value = withContext(Dispatchers.IO) { describeBackendStatus() }
                }
                StartingScreen(description)
            }
        }
    }

    // Go blocks on files and the network, so the backend is only ever called off the main thread.
    private fun describeBackendStatus(): String = try {
        val stateDirectory = File(noBackupFilesDir, "Hisar").apply { mkdirs() }
        val hisar = Hisarkit.new_(stateDirectory.path, "Hisar on Android", UnavailableSecretKeyStore, null, null, null)
        val status = backendJson.decodeFromString<VaultStatus>(hisar.status())
        Log.i(LOG_TAG, "HisarKit status: state=${status.state}")
        when {
            status.isUnlocked -> "Unlocked"
            status.isSignedIn -> "Locked"
            else -> "Signed out"
        }
    } catch (error: Exception) {
        Log.e(LOG_TAG, "HisarKit did not start", error)
        "Hisar could not start: ${error.message}"
    }

    private companion object {
        const val LOG_TAG = "Hisar"
    }
}

// The Keystore-backed store replaces this; until then nothing signs in, so no Secret Key is ever stored or read.
private object UnavailableSecretKeyStore : SecretKeyStore {
    private const val MESSAGE = "This build of Hisar cannot keep a Secret Key yet."

    override fun loadSecretKey(accountID: String): String = throw UnsupportedOperationException(MESSAGE)

    override fun storeSecretKey(accountID: String, secretKey: String) = throw UnsupportedOperationException(MESSAGE)

    override fun deleteSecretKey(accountID: String) = throw UnsupportedOperationException(MESSAGE)
}

@Composable
private fun StartingScreen(description: String?) {
    Column(
        modifier = Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background).safeDrawingPadding().padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("Hisar", style = MaterialTheme.typography.displaySmall, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.primary)
        Spacer(Modifier.height(8.dp))
        Text(
            description ?: "Starting…",
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
    }
}

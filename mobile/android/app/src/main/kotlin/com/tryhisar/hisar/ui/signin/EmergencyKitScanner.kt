package com.tryhisar.hisar.ui.signin

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.PhotoCamera
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.item.GlassTextButton
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import java.util.concurrent.Executors

/** Where the camera permission stands: asked for while the sheet opens, and read again each time Hisar returns. */
private enum class CameraAccess { ASKING, GRANTED, DENIED }

/**
 * The camera sheet that reads the QR code of an Emergency Kit, as the iPhone app's EmergencyKitScannerSheet: the text
 * of the first QR code it reads goes to [onScanned].
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EmergencyKitScannerSheet(onScanned: (String) -> Unit, onCancel: () -> Unit) {
    val context = LocalContext.current
    val isCameraAvailable = remember { context.packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY) }
    var access by remember { mutableStateOf(if (context.hasCameraPermission()) CameraAccess.GRANTED else CameraAccess.ASKING) }
    val permissionRequest = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { isGranted ->
        access = if (isGranted) CameraAccess.GRANTED else CameraAccess.DENIED
    }
    LaunchedEffect(Unit) {
        if (isCameraAvailable && access == CameraAccess.ASKING) permissionRequest.launch(Manifest.permission.CAMERA)
    }
    LifecycleResumeEffect(Unit) {
        if (context.hasCameraPermission()) access = CameraAccess.GRANTED
        onPauseOrDispose {}
    }

    ModalBottomSheet(
        onDismissRequest = onCancel,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = HisarTheme.colors.groupedBackground,
        dragHandle = null,
    ) {
        Box(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 12.dp),
        ) {
            Text("Scan Emergency Kit", style = HisarType.headline, color = HisarTheme.colors.label, textAlign = TextAlign.Center, modifier = Modifier.align(Alignment.Center))
            Row(Modifier.align(Alignment.CenterStart)) {
                GlassTextButton("Cancel", onClick = onCancel, testTag = "scanner-cancel")
            }
        }
        Box(Modifier.fillMaxSize().testTag("scanner")) {
            when {
                !isCameraAvailable -> ScannerNote("No camera is available. Type the details from your Emergency Kit instead.")
                access == CameraAccess.GRANTED -> {
                    QrCameraPreview(onScanned, Modifier.fillMaxSize())
                    Text(
                        "Point the camera at the QR code on your Emergency Kit.",
                        style = HisarType.subheadline,
                        color = HisarTheme.colors.label,
                        modifier = Modifier
                            .align(Alignment.BottomCenter)
                            .windowInsetsPadding(WindowInsets.navigationBars)
                            .padding(24.dp)
                            .background(HisarTheme.colors.cellBackground.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                            .padding(14.dp),
                    )
                }
                access == CameraAccess.DENIED -> ScannerNote(
                    "Hisar needs the camera to scan the QR code. Allow it in Settings, or type the details from your Emergency Kit instead.",
                ) {
                    PlainButton("Open Settings", { context.openAppSettings() }, testTag = "scanner-open-settings")
                }
                else -> Box(Modifier.fillMaxSize().background(Color.Black))
            }
        }
    }
}

@Composable
private fun ScannerNote(text: String, action: (@Composable () -> Unit)? = null) {
    Column(
        Modifier
            .fillMaxSize()
            .padding(32.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp, Alignment.CenterVertically),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(Icons.Filled.PhotoCamera, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(40.dp))
        Text(text, style = HisarType.body, color = HisarTheme.colors.secondaryLabel, textAlign = TextAlign.Center, modifier = Modifier.testTag("scanner-note"))
        action?.invoke()
    }
}

/** The back camera's picture, or the front one's on a device without one, while its frames go through [QrCodeAnalyzer]. */
@Composable
private fun QrCameraPreview(onScanned: (String) -> Unit, modifier: Modifier) {
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    val currentOnScanned by rememberUpdatedState(onScanned)
    val previewView = remember {
        PreviewView(context).apply {
            implementationMode = PreviewView.ImplementationMode.COMPATIBLE
            scaleType = PreviewView.ScaleType.FILL_CENTER
            setBackgroundColor(android.graphics.Color.BLACK)
        }
    }
    DisposableEffect(lifecycleOwner) {
        val mainExecutor = ContextCompat.getMainExecutor(context)
        val analysisExecutor = Executors.newSingleThreadExecutor()
        val preview = Preview.Builder().build().apply { setSurfaceProvider(previewView.surfaceProvider) }
        val analysis = ImageAnalysis.Builder().setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST).build().apply {
            setAnalyzer(analysisExecutor, QrCodeAnalyzer { text -> mainExecutor.execute { currentOnScanned(text) } })
        }
        val providerFuture = ProcessCameraProvider.getInstance(context)
        var boundProvider: ProcessCameraProvider? = null
        var isDisposed = false
        // Like the iPhone app, a camera that cannot start leaves the picture black rather than failing the sign-in screen.
        providerFuture.addListener({
            if (isDisposed) return@addListener
            val provider = runCatching { providerFuture.get() }.getOrNull() ?: return@addListener
            val selector = listOf(CameraSelector.DEFAULT_BACK_CAMERA, CameraSelector.DEFAULT_FRONT_CAMERA)
                .firstOrNull { runCatching { provider.hasCamera(it) }.getOrDefault(false) } ?: return@addListener
            provider.bindToLifecycle(lifecycleOwner, selector, preview, analysis)
            boundProvider = provider
        }, mainExecutor)
        onDispose {
            isDisposed = true
            boundProvider?.unbind(preview, analysis)
            analysis.clearAnalyzer()
            analysisExecutor.shutdown()
        }
    }
    AndroidView({ previewView }, modifier)
}

private fun Context.hasCameraPermission(): Boolean =
    ContextCompat.checkSelfPermission(this, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED

private fun Context.openAppSettings() {
    startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", packageName, null)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
}

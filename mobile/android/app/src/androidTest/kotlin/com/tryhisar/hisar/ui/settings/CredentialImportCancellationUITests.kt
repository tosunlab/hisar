package com.tryhisar.hisar.ui.settings

import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.test.core.app.ApplicationProvider
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.app.CredentialExchangeResult
import java.io.BufferedInputStream
import java.io.Closeable
import java.io.DataInputStream
import java.net.HttpURLConnection
import java.net.ServerSocket
import java.net.Socket
import java.net.URL
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class CredentialImportCancellationUITests : HisarUITestCase() {
    @Test
    fun testCancelAndBackStopActiveImportsWithoutLosingSavedBatches() {
        val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()
        ImportSaveBarrier(readArgument("HISAR_UI_TEST_SERVER")).use { proxy ->
            launch()
            signUp(
                readArgument("HISAR_UI_TEST_IMPORT_CANCEL_INVITE_LINK").replace(readArgument("HISAR_UI_TEST_SERVER"), proxy.origin),
                readArgument("HISAR_UI_TEST_IMPORT_CANCEL_EMAIL"),
                "Import cancellation",
            )
            val backend = application.backend
            val vaultID = backend.listAllVault().single { it.name == "Personal" }.id
            application.credentialExchangeReceiver = { CredentialExchangeResult.Received(credentials("First")) }
            openSettings("open-account")
            tap("open-credential-import")
            waitFor("credential-import-start")
            proxy.holdSave(11)
            tapWhenEnabled("credential-import-start")
            waitFor(hasText("10 of 35 saved"))
            proxy.awaitHeldSave()
            composeRule.onNode(hasTestTag("credential-import-cancel")).assertIsEnabled()
            tap("credential-import-cancel")
            waitForGone("credential-import")
            waitFor("open-credential-import")

            application.credentialExchangeReceiver = { CredentialExchangeResult.Received(credentials("Second")) }
            tap("open-credential-import")
            waitFor("credential-import-receiving")
            proxy.releaseSave()
            waitFor("credential-import-start")
            val firstItems = backend.listAllItem(vaultID)
            assertEquals("the in-flight batch finishes but no third batch starts", 20, firstItems.size)
            assertEquals((1..20).map { "First $it" }.toSet(), firstItems.map { it.title }.toSet())

            proxy.holdSave(31)
            tapWhenEnabled("credential-import-start")
            waitFor(hasText("10 of 35 saved"))
            proxy.awaitHeldSave()
            goBack()
            waitForGone("credential-import")
            waitFor("open-credential-import")
            proxy.releaseSave()
            val allItems = backend.listAllItem(vaultID)
            assertEquals(40, allItems.size)
            assertEquals((1..20).map { "Second $it" }.toSet(), allItems.filter { it.title.startsWith("Second ") }.map { it.title }.toSet())
            assertEquals("no later import batch is sent to the real server", 40, proxy.savedRequests.get())
        }
    }

    private fun credentials(prefix: String): ByteArray {
        val items = (1..35).joinToString(",") { index ->
            """{"id":"item-$index","title":"$prefix $index","credentials":[{"type":"basic-auth","username":{"fieldType":"string","value":"alice"},"password":{"fieldType":"concealed-string","value":"imported password $index"}}]}"""
        }
        return """{"version":{"major":1,"minor":0},"exporterDisplayName":"Cancellation fixture","exporterRpId":"fixture.test","timestamp":1767225600,"accounts":[{"id":"account","email":"alice@example.com","username":"alice","collections":[],"items":[$items]}]}""".encodeToByteArray()
    }
}

private class ImportSaveBarrier(private val upstream: String) : Closeable {
    private val listener = ServerSocket(0)
    private val workers = Executors.newCachedThreadPool()
    val origin = "http://127.0.0.1:${listener.localPort}"
    val savedRequests = AtomicInteger()
    @Volatile private var heldNumber = -1
    private var reached = CountDownLatch(1)
    private var release = CountDownLatch(1)

    init {
        workers.execute {
            while (!listener.isClosed) {
                val socket = try { listener.accept() } catch (_: java.net.SocketException) { break }
                workers.execute {
                    try {
                        socket.use { relay(it) }
                    } catch (error: java.io.IOException) {
                        if (!listener.isClosed) throw error
                    }
                }
            }
        }
    }

    fun holdSave(number: Int) {
        reached = CountDownLatch(1)
        release = CountDownLatch(1)
        heldNumber = number
    }

    fun awaitHeldSave() {
        assertTrue("a save in the second batch reaches the server proxy", reached.await(30, TimeUnit.SECONDS))
    }

    fun releaseSave() = release.countDown()

    private fun relay(socket: Socket) {
        val input = BufferedInputStream(socket.getInputStream())
        val request = readLine(input).split(' ')
        val headers = linkedMapOf<String, String>()
        while (true) {
            val line = readLine(input)
            if (line.isEmpty()) break
            headers[line.substringBefore(':').lowercase()] = line.substringAfter(':').trim()
        }
        val body = ByteArray(headers["content-length"]?.toInt() ?: 0)
        DataInputStream(input).readFully(body)
        if (request[0] == "PUT" && request[1].matches(Regex("/api/v1/vaults/[^/]+/items/[^/]+/import"))) {
            if (savedRequests.incrementAndGet() == heldNumber) {
                reached.countDown()
                check(release.await(45, TimeUnit.SECONDS)) { "the held item save was not released" }
            }
        }
        val connection = URL(upstream + request[1]).openConnection() as HttpURLConnection
        try {
            connection.requestMethod = request[0]
            for ((name, value) in headers) {
                if (name !in setOf("host", "connection", "content-length", "accept-encoding")) connection.setRequestProperty(name, value)
            }
            connection.setRequestProperty("Accept-Encoding", "identity")
            if (body.isNotEmpty()) {
                connection.doOutput = true
                connection.setFixedLengthStreamingMode(body.size)
                connection.outputStream.use { it.write(body) }
            }
            val status = connection.responseCode
            val output = socket.getOutputStream()
            output.write("HTTP/1.1 $status ${connection.responseMessage}\r\n".toByteArray())
            for ((name, values) in connection.headerFields) {
                if (name != null && name.lowercase() !in setOf("connection", "transfer-encoding")) {
                    for (value in values) output.write("$name: $value\r\n".toByteArray())
                }
            }
            output.write("Connection: close\r\n\r\n".toByteArray())
            output.flush()
            val response = if (status >= 400) connection.errorStream else connection.inputStream
            response?.use { stream ->
                val buffer = ByteArray(8192)
                while (true) {
                    val count = stream.read(buffer)
                    if (count < 0) break
                    output.write(buffer, 0, count)
                    output.flush()
                }
            }
        } finally {
            connection.disconnect()
        }
    }

    private fun readLine(input: BufferedInputStream): String = buildString {
        while (true) {
            val next = input.read()
            check(next >= 0) { "the HTTP request ended before its headers" }
            if (next == 10) break
            if (next != 13) append(next.toChar())
        }
    }

    override fun close() {
        releaseSave()
        listener.close()
        workers.shutdown()
    }
}

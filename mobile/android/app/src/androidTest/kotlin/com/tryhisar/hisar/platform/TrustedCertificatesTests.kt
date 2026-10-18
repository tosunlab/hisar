package com.tryhisar.hisar.platform

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.tryhisar.hisar.backend.createTestBackend
import com.tryhisar.hisar.model.BackendError
import java.io.Closeable
import java.net.InetAddress
import java.security.KeyFactory
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.security.spec.PKCS8EncodedKeySpec
import java.util.Base64
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit
import javax.net.ssl.KeyManagerFactory
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLServerSocket
import javax.net.ssl.SSLSocket
import kotlin.concurrent.thread
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class TrustedCertificatesTests {
    @Test
    fun testEveryCertificateTheDeviceTrustsIsPassedToGo() {
        val store = KeyStore.getInstance("AndroidCAStore").apply { load(null) }
        val allTrusted = store.aliases().asSequence().map { store.getCertificate(it) }.toSet()
        val allPassed = CertificateFactory.getInstance("X.509").generateCertificates(TrustedCertificates.readPem().byteInputStream()).toSet()
        assertTrue("the device trusts ${allTrusted.size} certificates", allTrusted.size > 100)
        assertEquals(allTrusted, allPassed)
    }

    @Test
    fun testPrivateCAServerIsReachedOnlyWithItsCertificate() {
        PrivateCAServer().use { server ->
            val inviteLink = "https://127.0.0.1:${server.port}/invite/private-ca-test"
            val deviceRoots = TrustedCertificates.readPem()

            val withDeviceRoots = createTestBackend(trustedCertificates = deviceRoots)
            assertThrows(BackendError::class.java) { withDeviceRoots.signUp(inviteLink, "tls@example.com", "TLS", "a long master password") }
            val refused = server.takeConnection()
            assertTrue(refused, refused.startsWith("handshake failed"))
            server.forgetConnections()

            val withPrivateCA = createTestBackend(trustedCertificates = deviceRoots + readAsset("ca.pem"))
            assertThrows(BackendError::class.java) { withPrivateCA.signUp(inviteLink, "tls@example.com", "TLS", "a long master password") }
            val reached = server.takeConnection()
            assertTrue(reached, reached.startsWith("request "))
        }
    }

    /** An HTTPS server on the device whose certificate a private CA signed; it answers every request with 404. */
    private class PrivateCAServer : Closeable {
        private val socket: SSLServerSocket
        private val allConnection = LinkedBlockingQueue<String>()
        val port: Int get() = socket.localPort

        init {
            val factory = CertificateFactory.getInstance("X.509")
            val chain = arrayOf(readAsset("server.pem"), readAsset("ca.pem")).map { factory.generateCertificate(it.byteInputStream()) as X509Certificate }
            val keyDer = Base64.getMimeDecoder().decode(readAsset("server-key.pem").lines().filterNot { it.startsWith("-----") }.joinToString(""))
            val key = KeyFactory.getInstance("EC").generatePrivate(PKCS8EncodedKeySpec(keyDer))
            val keyStore = KeyStore.getInstance(KeyStore.getDefaultType()).apply {
                load(null)
                setKeyEntry("server", key, CharArray(0), chain.toTypedArray())
            }
            val keyManagers = KeyManagerFactory.getInstance(KeyManagerFactory.getDefaultAlgorithm()).apply { init(keyStore, CharArray(0)) }.keyManagers
            val context = SSLContext.getInstance("TLS").apply { init(keyManagers, null, null) }
            socket = context.serverSocketFactory.createServerSocket(0, 10, InetAddress.getByName("127.0.0.1")) as SSLServerSocket
            thread(isDaemon = true) { serve() }
        }

        /** What the next connection did: "handshake failed: …" or "request GET /…". */
        fun takeConnection(): String = allConnection.poll(60, TimeUnit.SECONDS) ?: "no connection"

        fun forgetConnections() = allConnection.clear()

        private fun serve() {
            while (!socket.isClosed) {
                val connection = try {
                    socket.accept() as SSLSocket
                } catch (_: Exception) {
                    return
                }
                connection.use {
                    try {
                        it.soTimeout = 10_000
                        it.startHandshake()
                        val requestLine = it.inputStream.bufferedReader().readLine().orEmpty()
                        allConnection.add("request $requestLine")
                        it.outputStream.write("HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n".toByteArray())
                        it.outputStream.flush()
                    } catch (error: Exception) {
                        allConnection.add("handshake failed: $error")
                    }
                }
            }
        }

        override fun close() = socket.close()
    }

    private companion object {
        fun readAsset(name: String): String =
            InstrumentationRegistry.getInstrumentation().context.assets.open("private-ca/$name").bufferedReader().use { it.readText() }
    }
}

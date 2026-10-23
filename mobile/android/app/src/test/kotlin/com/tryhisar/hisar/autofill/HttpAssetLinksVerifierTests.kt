package com.tryhisar.hisar.autofill

import java.io.ByteArrayInputStream
import java.io.IOException
import java.io.InputStream
import java.net.HttpURLConnection
import java.net.SocketTimeoutException
import java.net.URL
import java.net.URLConnection
import java.net.URLStreamHandler
import java.net.URLStreamHandlerFactory
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.*

class HttpAssetLinksVerifierTests {
    private data class Response(val status: Int = 200, val body: String = "[]", val error: IOException? = null)
    private companion object {
        val responseByHost = ConcurrentHashMap<String, Response>()
        val countByHost = ConcurrentHashMap<String, AtomicInteger>()
        val app = AndroidApp("com.example.app", "AB:CD")
        val positive = """[{"relation":["delegate_permission/common.get_login_creds"],"target":{"namespace":"android_app","package_name":"com.example.app","sha256_cert_fingerprints":["AB:CD"]}}]"""
        init {
            URL.setURLStreamHandlerFactory(URLStreamHandlerFactory { protocol ->
                if (protocol != "https") null else object : URLStreamHandler() {
                    override fun openConnection(url: URL): URLConnection {
                        countByHost.computeIfAbsent(url.host) { AtomicInteger() }.incrementAndGet()
                        val response = responseByHost.getValue(url.host)
                        return object : HttpURLConnection(url) {
                            override fun connect() {}
                            override fun disconnect() {}
                            override fun usingProxy() = false
                            override fun getResponseCode(): Int {
                                response.error?.let { throw it }
                                return response.status
                            }
                            override fun getInputStream(): InputStream = ByteArrayInputStream(response.body.toByteArray())
                        }
                    }
                }
            })
        }
    }

    private fun assertRecovery(host: String, response: Response) {
        responseByHost[host] = response
        assertFalse(HttpAssetLinksVerifier.isVouchingFor(host, app))
        responseByHost[host] = Response(body = positive)
        assertTrue(HttpAssetLinksVerifier.isVouchingFor(host, app), "a transient failure was cached")
        assertEquals(2, countByHost.getValue(host).get())
    }

    @Test fun testHttpServerFailuresAreRetriedAfterRecovery() {
        for (status in listOf(500, 502, 503, 504)) assertRecovery("http$status.example", Response(status = status))
    }

    @Test fun testTransportFailureIsRetried() = assertRecovery("transport.example", Response(error = IOException("injected failure")))

    @Test fun testTimeoutIsRetried() = assertRecovery("timeout.example", Response(error = SocketTimeoutException("injected timeout")))

    @Test fun testRevokedPositiveRemainsCached() {
        val host = "revoked.example"
        responseByHost[host] = Response(body = positive)
        assertTrue(HttpAssetLinksVerifier.isVouchingFor(host, app))
        responseByHost[host] = Response()
        assertTrue(HttpAssetLinksVerifier.isVouchingFor(host, app))
        assertEquals(1, countByHost.getValue(host).get())
    }

    @Test fun testDefinitiveDenialRemainsCached() {
        val host = "denied.example"
        responseByHost[host] = Response(status = 404)
        assertFalse(HttpAssetLinksVerifier.isVouchingFor(host, app))
        responseByHost[host] = Response(body = positive)
        assertFalse(HttpAssetLinksVerifier.isVouchingFor(host, app))
        assertEquals(1, countByHost.getValue(host).get())
    }
}

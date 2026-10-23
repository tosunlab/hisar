package com.tryhisar.hisar.autofill

import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class AssetLinksReaderTests {
    private val allURL = (1..10).map { "https://host$it.example.com/.well-known/assetlinks.json" }

    @Test
    fun testRequestWaitsUntilItsDeadlineAndSitesItCutOffAnswerTheNextRequest() {
        val release = CountDownLatch(1)
        val twoStarted = CountDownLatch(2)
        val fetchCountByURL = ConcurrentHashMap<String, AtomicInteger>()
        val running = AtomicInteger()
        val maximumRunning = AtomicInteger()
        val reader = AssetLinksReader(threadCount = 2) { url ->
            fetchCountByURL.getOrPut(url) { AtomicInteger() }.incrementAndGet()
            maximumRunning.accumulateAndGet(running.incrementAndGet()) { a, b -> maxOf(a, b) }
            twoStarted.countDown()
            try {
                check(release.await(10, TimeUnit.SECONDS))
                "statements of $url"
            } finally {
                running.decrementAndGet()
            }
        }

        val startedAt = System.nanoTime()
        assertEquals(List(10) { "" }, reader.readAll(allURL, timeoutMillis = 200))
        assertTrue(TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - startedAt) < 2_000, "the request ends at its deadline, not when its sites answer")
        assertEquals(List(10) { "" }, reader.readAll(allURL, timeoutMillis = 100), "a second request waits on the same reads")
        assertTrue(twoStarted.await(10, TimeUnit.SECONDS))
        assertEquals(2, fetchCountByURL.size, "two threads, so eight sites are still queued")

        release.countDown()
        assertEquals(allURL.map { "statements of $it" }, reader.readAll(allURL, timeoutMillis = 5_000), "the sites the deadline cut off answer the next request")
        assertEquals(allURL.toSet(), fetchCountByURL.keys)
        assertTrue(fetchCountByURL.values.all { it.get() == 1 }, "each site is asked once, however many requests waited on it")
        assertEquals(2, maximumRunning.get())
    }

    @Test
    fun testSiteThatCouldNotBeAskedIsAskedAgain() {
        val fetchCount = AtomicInteger()
        val reader = AssetLinksReader(threadCount = 2) { if (fetchCount.incrementAndGet() == 1) null else "[]" }
        val url = allURL.take(1)
        assertEquals(listOf(""), reader.readAll(url, timeoutMillis = 5_000))
        assertEquals(listOf("[]"), reader.readAll(url, timeoutMillis = 5_000))
        assertEquals("[]", reader.read(url.single()))
        assertEquals(2, fetchCount.get(), "an answer is kept, a failure is not")
    }
}

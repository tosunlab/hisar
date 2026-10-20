package com.tryhisar.hisar.autofill

import java.util.concurrent.*
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.*

class AssetLinksResourceTests {
    private fun AssetLinksReader.field(name: String): Any = javaClass.getDeclaredField(name).let { it.isAccessible = true; it.get(this) }
    private fun AssetLinksReader.stop() { (field("executor") as ThreadPoolExecutor).shutdownNow() }

    @Test fun testDeadlinesCannotLeaveAnUnboundedQueueOrTrackedReads() {
        val release = CountDownLatch(1)
        val started = CountDownLatch(2)
        val reader = AssetLinksReader(2) {
            started.countDown()
            check(release.await(10, TimeUnit.SECONDS))
            "[]"
        }
        val urls = (1..4_000).map { "https://host$it.example/assetlinks.json" }
        try {
            assertEquals(List(3_000) { "" }, reader.readAll(urls.take(3_000), 0))
            assertTrue(started.await(10, TimeUnit.SECONDS))
            reader.readAll(urls.takeLast(1_000), 0)
            assertTrue((reader.field("executor") as ThreadPoolExecutor).queue.size <= 64, "expired requests left unbounded queued work")
            assertTrue((reader.field("readByURL") as Map<*, *>).size <= 66, "overload retained unbounded addresses")
            release.countDown()
            assertEquals("[]", reader.read(urls.last()), "an overloaded address can be asked by a later request")
        } finally { release.countDown(); reader.stop() }
    }

    @Test fun testAnswerCacheEvictsOldEntriesAtItsCapacity() {
        val counts = ConcurrentHashMap<String, AtomicInteger>()
        val reader = AssetLinksReader(2) { url -> counts.computeIfAbsent(url) { AtomicInteger() }.incrementAndGet(); "[]" }
        try {
            repeat(1_000) { reader.read("https://host$it.example/assetlinks.json") }
            assertTrue((reader.field("answerByURL") as Map<*, *>).size <= 128, "answers exceeded the cache capacity")
            reader.read("https://host0.example/assetlinks.json")
            assertEquals(2, counts.getValue("https://host0.example/assetlinks.json").get())
        } finally { reader.stop() }
    }

    @Test fun testExpiredAnswersAreEvictedWhenOtherAddressesAreRead() {
        val reader = AssetLinksReader(2) { "[]" }
        try {
            repeat(100) { reader.read("https://old$it.example/assetlinks.json") }
            @Suppress("UNCHECKED_CAST")
            val answers = reader.field("answerByURL") as MutableMap<String, Any>
            val constructor = answers.values.first().javaClass.declaredConstructors.single().apply { isAccessible = true }
            val oldTimestamp = System.currentTimeMillis() - TimeUnit.HOURS.toMillis(2)
            answers.keys.toList().forEach { answers[it] = constructor.newInstance("[]", oldTimestamp) }
            reader.read("https://new.example/assetlinks.json")
            assertEquals(1, answers.size, "expired answers remain stored")
        } finally { reader.stop() }
    }

    @Test fun testDirectNativeReadsShareTheWorkerLimitAndSingleFlight() {
        val callers = Executors.newFixedThreadPool(8)
        val ready = CountDownLatch(8)
        val firstFetch = CountDownLatch(1)
        val secondFetch = CountDownLatch(2)
        val release = CountDownLatch(1)
        val fetches = AtomicInteger()
        val reader = AssetLinksReader(2) {
            fetches.incrementAndGet()
            firstFetch.countDown()
            secondFetch.countDown()
            check(release.await(10, TimeUnit.SECONDS))
            "[]"
        }
        try {
            val futures = (1..8).map { callers.submit<String> { ready.countDown(); ready.await(); reader.read("https://same.example/assetlinks.json") } }
            assertTrue(firstFetch.await(10, TimeUnit.SECONDS))
            assertFalse(secondFetch.await(200, TimeUnit.MILLISECONDS), "direct reads bypassed single flight")
            release.countDown()
            futures.forEach { assertEquals("[]", it.get(10, TimeUnit.SECONDS)) }
            assertEquals(1, fetches.get())
        } finally { release.countDown(); callers.shutdownNow(); reader.stop() }
    }

    @Test fun testLaterCandidatesAreAdmittedAsTheBoundedQueueDrains() {
        val reader = AssetLinksReader(2) { it }
        try {
            val urls = (1..502).map { "https://host$it.example/assetlinks.json" }
            assertEquals(urls, reader.readAll(urls, 5_000))
        } finally { reader.stop() }
    }

    @Test fun testSlowFirstCandidateDoesNotStarveOverloadedCandidates() {
        val release = CountDownLatch(1)
        val slow = "https://slow.example/assetlinks.json"
        val reader = AssetLinksReader(2) { url ->
            if (url == slow) {
                check(release.await(10, TimeUnit.SECONDS))
                null
            } else {
                Thread.sleep(1)
                url
            }
        }
        try {
            val urls = listOf(slow) + (1..100).map { "https://host$it.example/assetlinks.json" }
            assertEquals(listOf("") + urls.drop(1), reader.readAll(urls, 1_000), "fast candidates were left out although a worker was idle")
        } finally { release.countDown(); reader.stop() }
    }

    @Test fun testOverloadedRequestWaitsForCapacityInsteadOfPollingRejections() {
        val release = CountDownLatch(1)
        val started = CountDownLatch(1)
        val reader = AssetLinksReader(1) {
            started.countDown()
            check(release.await(10, TimeUnit.SECONDS))
            "[]"
        }
        try {
            reader.readAll((1..65).map { "https://busy$it.example/assetlinks.json" }, 0)
            assertTrue(started.await(10, TimeUnit.SECONDS))
            val executor = reader.field("executor") as ThreadPoolExecutor
            val rejections = AtomicInteger()
            executor.rejectedExecutionHandler = RejectedExecutionHandler { _, _ ->
                rejections.incrementAndGet()
                throw RejectedExecutionException()
            }
            val urls = (1..100).map { "https://waiting$it.example/assetlinks.json" }
            assertEquals(List(100) { "" }, reader.readAll(urls, 200))
            assertTrue(rejections.get() <= 200, "the full queue was polled repeatedly without any capacity change")
        } finally { release.countDown(); reader.stop() }
    }

    @Test fun testDirectReadsOfDifferentSitesShareTheWorkerLimit() {
        val callers = Executors.newFixedThreadPool(8)
        val started = CountDownLatch(2)
        val thirdFetch = CountDownLatch(3)
        val release = CountDownLatch(1)
        val reader = AssetLinksReader(2) {
            started.countDown()
            thirdFetch.countDown()
            check(release.await(10, TimeUnit.SECONDS))
            "[]"
        }
        try {
            val futures = (1..8).map { index -> callers.submit<String> { reader.read("https://host$index.example/assetlinks.json") } }
            assertTrue(started.await(10, TimeUnit.SECONDS))
            assertFalse(thirdFetch.await(200, TimeUnit.MILLISECONDS), "direct reads bypassed the worker limit")
            release.countDown()
            futures.forEach { assertEquals("[]", it.get(10, TimeUnit.SECONDS)) }
        } finally { release.countDown(); callers.shutdownNow(); reader.stop() }
    }

    @Test fun testInterruptingAWaiterDoesNotCancelTheReadSharedWithAnotherRequest() {
        val caller = Executors.newSingleThreadExecutor()
        val started = CountDownLatch(1)
        val release = CountDownLatch(1)
        val fetches = AtomicInteger()
        val reader = AssetLinksReader(2) {
            fetches.incrementAndGet()
            started.countDown()
            check(release.await(10, TimeUnit.SECONDS))
            "[]"
        }
        try {
            val request = caller.submit<String> { reader.read("https://shared.example/assetlinks.json") }
            assertTrue(started.await(10, TimeUnit.SECONDS))
            request.cancel(true)
            release.countDown()
            assertEquals("[]", reader.read("https://shared.example/assetlinks.json"))
            assertEquals(1, fetches.get())
        } finally { release.countDown(); caller.shutdownNow(); reader.stop() }
    }

    @Test fun testDefinitiveNegativeAndPositiveAnswersKeepTheirLifetime() {
        var statement = ""
        val fetches = AtomicInteger()
        val reader = AssetLinksReader(2) { fetches.incrementAndGet(); statement }
        try {
            val app = AndroidApp("com.example.app", "AB:CD")
            val positive = """[{"relation":["delegate_permission/common.get_login_creds"],"target":{"namespace":"android_app","package_name":"com.example.app","sha256_cert_fingerprints":["AB:CD"]}}]"""
            assertEquals("", reader.read("https://negative.example/assetlinks.json"))
            statement = positive
            assertEquals("", reader.read("https://negative.example/assetlinks.json"))
            assertTrue(AssetLinks.isVouchingFor(reader.read("https://positive.example/assetlinks.json"), app))
            statement = "[]"
            assertTrue(AssetLinks.isVouchingFor(reader.read("https://positive.example/assetlinks.json"), app))
            assertEquals(2, fetches.get())
        } finally { reader.stop() }
    }
}

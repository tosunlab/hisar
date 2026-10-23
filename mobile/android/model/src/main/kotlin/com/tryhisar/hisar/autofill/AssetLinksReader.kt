package com.tryhisar.hisar.autofill

import java.util.concurrent.ArrayBlockingQueue
import java.util.concurrent.CompletableFuture
import java.util.concurrent.ExecutionException
import java.util.concurrent.Future
import java.util.concurrent.FutureTask
import java.util.concurrent.RejectedExecutionException
import java.util.concurrent.ThreadPoolExecutor
import java.util.concurrent.TimeUnit
import java.util.concurrent.TimeoutException
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock

/**
 * Reads statement files on shared workers, with at most 64 queued reads and 128 cached answers. Each address has at
 * most one outstanding read. Admitted reads finish after a caller's deadline, so their answers can serve another
 * request. Rejected addresses are retried between short waits on admitted reads. When none are admitted, the caller
 * waits for queue capacity instead of polling. Overload is not cached, and a later request can try again.
 *
 * [fetch] returns null for transient failures, which are not cached. Definitive answers expire after an hour.
 */
class AssetLinksReader(threadCount: Int, private val fetch: (url: String) -> String?) {
    private class Answer(val statementJSON: String, val fetchedAt: Long)

    private val monitor = ReentrantLock()
    private val capacityChanged = monitor.newCondition()
    private val answerByURL = LinkedHashMap<String, Answer>(MAXIMUM_ANSWERS, 0.75f, true)
    private val readByURL = HashMap<String, Future<String>>()
    private val executor = ThreadPoolExecutor(threadCount, threadCount, 30, TimeUnit.SECONDS, ArrayBlockingQueue(MAXIMUM_QUEUED_READS)) { task ->
        Thread(task, "AssetLinksReader").apply { isDaemon = true }
    }.apply { allowCoreThreadTimeOut(true) }

    /** Answers in input order, or "" for addresses that did not answer before the shared deadline. */
    fun readAll(allURL: List<String>, timeoutMillis: Long): List<String> {
        val deadline = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(timeoutMillis)
        val answers = MutableList(allURL.size) { "" }
        val reads = Array(allURL.size) { start(allURL[it]) }
        val pending = allURL.indices.filterTo(ArrayList()) { reads[it] == null }
        do {
            var waiting: Future<String>? = null
            for ((index, read) in reads.withIndex()) {
                if (read == null) continue
                if (!read.isDone) {
                    if (waiting == null) waiting = read
                    continue
                }
                try {
                    answers[index] = read.get()
                } catch (_: ExecutionException) {
                }
                reads[index] = null
            }
            if (pending.isEmpty() && waiting == null) break
            val remaining = deadline - System.nanoTime()
            if (remaining <= 0) break
            val slice = minOf(remaining, TimeUnit.MILLISECONDS.toNanos(25))
            if (waiting != null) {
                try {
                    waiting.get(slice, TimeUnit.NANOSECONDS)
                } catch (_: TimeoutException) {
                } catch (_: ExecutionException) {
                }
            } else {
                monitor.withLock {
                    if (executor.queue.remainingCapacity() == 0) capacityChanged.awaitNanos(remaining)
                }
            }
            if (System.nanoTime() < deadline) {
                val candidates = pending.iterator()
                while (candidates.hasNext()) {
                    val index = candidates.next()
                    val read = start(allURL[index]) ?: continue
                    reads[index] = read
                    candidates.remove()
                }
            }
        } while (true)
        return answers
    }

    /** Native passkey checks use the same deadline, workers, cache and single-flight map as password matching. */
    fun read(url: String): String = readAll(listOf(url), READ_TIMEOUT_MILLISECONDS).single()

    private fun start(url: String): Future<String>? = monitor.withLock {
        val now = System.currentTimeMillis()
        answerByURL.entries.removeAll { now - it.value.fetchedAt >= CACHE_MILLISECONDS }
        answerByURL[url]?.let { return@withLock CompletableFuture.completedFuture(it.statementJSON) }
        readByURL[url]?.let { return@withLock it }
        lateinit var task: FutureTask<String>
        task = FutureTask {
            monitor.withLock { capacityChanged.signalAll() }
            try {
                val statementJSON = fetch(url)
                if (statementJSON != null) monitor.withLock {
                    answerByURL[url] = Answer(statementJSON, System.currentTimeMillis())
                    while (answerByURL.size > MAXIMUM_ANSWERS) answerByURL.remove(answerByURL.keys.first())
                }
                statementJSON.orEmpty()
            } finally {
                monitor.withLock {
                    readByURL.remove(url, task)
                    capacityChanged.signalAll()
                }
            }
        }
        readByURL[url] = task
        try {
            executor.execute(task)
            task
        } catch (_: RejectedExecutionException) {
            readByURL.remove(url, task)
            null
        }
    }

    private companion object {
        const val MAXIMUM_QUEUED_READS = 64
        const val MAXIMUM_ANSWERS = 128
        const val READ_TIMEOUT_MILLISECONDS = 4_000L
        const val CACHE_MILLISECONDS = 60 * 60 * 1000L
    }
}

package com.vanillapay.monitor

import android.app.Activity
import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.os.Looper
import androidx.core.content.FileProvider
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.Implementation
import org.robolectric.annotation.Implements
import org.robolectric.annotation.LooperMode
import org.robolectric.shadows.ShadowActivity.IntentForResult
import org.robolectric.shadows.ShadowContentResolver
import org.robolectric.shadow.api.ShadowPicker
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.IOException
import java.io.OutputStream
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit
import kotlin.coroutines.CoroutineContext

@RunWith(RobolectricTestRunner::class)
@Config(application = MonitorApp::class, sdk = [35],
    shadows = [ShadowMonitorAppStartup::class, WindowsFileProviderPathShadow::class])
@LooperMode(LooperMode.Mode.PAUSED)
class DiagnosticLogFilesTest {
    private lateinit var app: MonitorApp
    private lateinit var files: DiagnosticLogFiles
    private val actions get() = app.diagnosticLogActions
    private val history get() = app.connectionDiagnostics
    private val cache get() = File(app.cacheDir, "diagnostic-log-shares")
    private val document = Uri.parse("content://diagnostic-test-documents/selected")
    private val resolver get() = shadowOf(app.contentResolver)

    @Before fun setUp() {
        app = RuntimeEnvironment.getApplication() as MonitorApp
        files = DiagnosticLogFiles(app)
    }

    @After fun tearDown() {
        app.scope.cancel()
    }

    @Test fun shareUsesRealFileProviderAndGrantsOnlyReadAccess() {
        history.record(DiagnosticEvent.LISTENER_CONNECTED)
        var chooser: Intent? = null
        actions.share(app.scope) { chooser = it }
        waitFor { !actions.busy }

        val outer = requireNotNull(chooser)
        val send = requireNotNull(outer.getParcelableExtra(Intent.EXTRA_INTENT, Intent::class.java))
        val uri = requireNotNull(send.getParcelableExtra(Intent.EXTRA_STREAM, Uri::class.java))
        assertEquals(Intent.ACTION_CHOOSER, outer.action)
        assertEquals(Intent.ACTION_SEND, send.action)
        assertEquals("text/plain", send.type)
        assertFalse(send.hasExtra(Intent.EXTRA_TEXT))
        assertEquals("content", uri.scheme)
        assertEquals("${app.packageName}.diagnostic-log-files", uri.authority)
        assertTrue(requireNotNull(uri.lastPathSegment).endsWith(".txt"))
        assertEquals(uri, send.clipData!!.getItemAt(0).uri)
        assertEquals(uri, outer.clipData!!.getItemAt(0).uri)
        assertEquals(Intent.FLAG_GRANT_READ_URI_PERMISSION, send.flags)
        assertEquals(Intent.FLAG_GRANT_READ_URI_PERMISSION, outer.flags)

        val provider = ShadowContentResolver.getProvider(uri) as DiagnosticLogFileProvider
        val info = app.packageManager.resolveContentProvider(requireNotNull(uri.authority), 0)!!
        assertFalse(info.exported)
        assertTrue(info.grantUriPermissions)
        assertEquals("text/plain", provider.getType(uri))
        assertArrayEquals(snapshotBytes(), app.contentResolver.openInputStream(uri)!!.use { it.readBytes() })

        val outside = File(app.cacheDir, "outside.txt").apply { writeText("outside") }
        assertThrows(IllegalArgumentException::class.java) {
            FileProvider.getUriForFile(app, requireNotNull(uri.authority), outside)
        }
        val escaped = uri.buildUpon().path("diagnostic-logs/../outside.txt").build()
        assertThrows(SecurityException::class.java) { provider.openFile(escaped, "r") }
    }

    @Test fun pickerResultSurvivesActivityRecreationAndReadsTheLatestSnapshot() {
        history.record(DiagnosticEvent.SERVICE_STARTED)
        val before = snapshotBytes()
        val stream = object : ByteArrayOutputStream() {
            var closed = false
            override fun close() { closed = true; super.close() }
        }
        resolver.registerOutputStream(document, stream)
        val saved = Bundle()
        val launched = startActivity().use { controller ->
            val original = controller.get()
            val picker = launchPicker(original)
            original.exportDiagnosticLogs()
            assertNull(shadowOf(original).peekNextStartedActivityForResult())
            controller.stop().saveInstanceState(saved)
            picker
        }

        startActivity(saved).use { controller ->
            val recreated = controller.get()
            recreated.exportDiagnosticLogs()
            assertNull(shadowOf(recreated).peekNextStartedActivityForResult())
            history.record(DiagnosticEvent.LISTENER_CONNECTED)
            val after = snapshotBytes()
            assertFalse(before.contentEquals(after))

            result(recreated, launched, document)
            assertTrue(actions.busy)
            waitFor { !actions.busy }
            assertTrue(stream.closed)
            assertArrayEquals(after, stream.toByteArray())
            assertEquals("已导出日志文件。", actions.message)
            launchPicker(recreated)
        }
    }

    @Test fun cancellingPickerResetsPendingWithoutOpeningTheDocument() {
        var opens = 0
        resolver.registerOutputStreamSupplier(document) { opens++; ByteArrayOutputStream() }
        startActivity().use { controller ->
            val activity = controller.get()
            val launched = launchPicker(activity)
            deliverResult(activity, launched, Activity.RESULT_CANCELED)
            assertFalse(actions.busy)
            assertEquals("已取消导出。", actions.message)
            assertEquals(0, opens)
            launchPicker(activity)
        }
    }

    @Test fun emptySnapshotAfterPickerReturnsDoesNotOpenTheDocument() {
        var opens = 0
        resolver.registerOutputStreamSupplier(document) { opens++; ByteArrayOutputStream() }
        history.record(DiagnosticEvent.SERVICE_STARTED)
        startActivity().use { controller ->
            val activity = controller.get()
            val launched = launchPicker(activity)
            assertTrue(runBlocking { history.clear() })
            result(activity, launched, document)
            waitFor { !actions.busy }
            assertEquals(0, opens)
            assertEquals("暂无可导出的诊断日志。", actions.message)
            launchPicker(activity)
        }
    }

    @Test fun pickerRejectsInvalidDocumentUrisWithoutOpeningAStreamAndAllowsRetry() {
        history.record(DiagnosticEvent.SERVICE_STARTED)
        val invalidUris = listOf(Uri.parse("file:///diagnostics.txt"), Uri.parse("content:///diagnostics.txt"))
        var opens = 0
        invalidUris.forEach { uri ->
            resolver.registerOutputStreamSupplier(uri) { opens++; ByteArrayOutputStream() }
        }
        val valid = ByteArrayOutputStream()
        resolver.registerOutputStream(document, valid)
        startActivity().use { controller ->
            val activity = controller.get()
            for (uri in invalidUris) {
                result(activity, launchPicker(activity), uri)
                waitFor { !actions.busy }
                assertEquals("无法导出日志文件，请重试。", actions.message)
            }
            assertEquals(0, opens)
            result(activity, launchPicker(activity), document)
            waitFor { !actions.busy }
            assertEquals("已导出日志文件。", actions.message)
            assertArrayEquals(snapshotBytes(), valid.toByteArray())
        }
    }

    @Test fun exportReportsProviderOpenWriteAndCloseFailures() {
        history.record(DiagnosticEvent.SERVICE_STARTED)
        val failure = "无法导出日志文件，请重试。"
        resolver.registerOutputStreamSupplier(document) { throw SecurityException("permission revoked") }
        actions.export(document)
        waitFor { !actions.busy }
        assertEquals(failure, actions.message)

        resolver.registerOutputStream(document, object : OutputStream() {
            override fun write(value: Int) { throw IOException("write failed") }
        })
        actions.export(document)
        waitFor { !actions.busy }
        assertEquals(failure, actions.message)

        val closeFailure = object : ByteArrayOutputStream() {
            override fun close() { throw IOException("close failed") }
        }
        resolver.registerOutputStream(document, closeFailure)
        actions.export(document)
        waitFor { !actions.busy }
        assertArrayEquals(snapshotBytes(), closeFailure.toByteArray())
        assertEquals(failure, actions.message)
    }

    @Test fun retentionOnlyPrunesExpiredAndExcessShareFiles() {
        assertEquals(8, DiagnosticLogFiles.MAX_SHARED_FILES)
        assertEquals(TimeUnit.HOURS.toMillis(24), DiagnosticLogFiles.SHARE_TTL_MS)
        val now = System.currentTimeMillis()
        val expired = shareFixture("expired", now - TimeUnit.HOURS.toMillis(24))
        val recent = List(10) { index -> shareFixture("recent-$index", now - TimeUnit.MINUTES.toMillis(10L - index)) }
        val unrelated = File(cache, "keep.txt").apply { writeText("unrelated"); assertTrue(setLastModified(expired.lastModified())) }
        val sibling = File(app.cacheDir, expired.name).apply { writeText("outside cache directory") }
        files.prune(now)
        assertFalse(expired.exists())
        assertEquals(recent.drop(2).toSet(), recent.filter(File::exists).toSet())
        assertTrue(unrelated.exists())
        assertTrue(sibling.exists())

        val first = files.createShare()
        files.finishShare(first, handedOff = true)
        val second = files.createShare()
        files.finishShare(second, handedOff = true)
        assertTrue(first.exists())
        assertTrue(second.exists())
        assertNotEquals(first, second)
        assertEquals(8, cache.listFiles()!!.count { it.name.startsWith(DiagnosticLogFiles.SHARE_PREFIX) })
    }

    @Test fun chooserLaunchFailureRemovesOnlyTheNewFile() {
        history.record(DiagnosticEvent.SERVICE_STARTED)
        val now = System.currentTimeMillis()
        val previous = List(8) { index -> shareFixture("previous-$index", now - index * 1_000L) }.toSet()
        actions.share(app.scope) {
            assertEquals(9, cache.listFiles()!!.size)
            throw ActivityNotFoundException()
        }
        waitFor { !actions.busy }
        assertEquals(previous, cache.listFiles()!!.toSet())
        assertEquals("无法分享日志文件，请重试。", actions.message)
    }

    @Test fun cancellationAfterIoDiscardsThePendingShareWithoutOpeningChooser() {
        history.record(DiagnosticEvent.SERVICE_STARTED)
        val previous = shareFixture("previous", System.currentTimeMillis())
        val queue = LinkedBlockingQueue<Runnable>()
        val scope = CoroutineScope(SupervisorJob() + object : CoroutineDispatcher() {
            override fun dispatch(context: CoroutineContext, block: Runnable) { queue.add(block) }
        })
        fun continuation() = requireNotNull(queue.poll(5, TimeUnit.SECONDS))
        try {
            var presented = false
            actions.share(scope) { presented = true }
            var resumeIo = continuation()
            if (cache.listFiles()!!.size == 1) {
                // The snapshot suspended before the file operation began.
                resumeIo.run()
                resumeIo = continuation()
            }
            assertEquals(2, cache.listFiles()!!.size)
            assertTrue(actions.busy)
            scope.cancel()
            resumeIo.run()
            continuation().run()
            assertFalse(presented)
            assertFalse(actions.busy)
            assertEquals(listOf(previous), cache.listFiles()!!.toList())
        } finally {
            scope.cancel()
        }
    }

    private fun startActivity(saved: Bundle? = null) =
        Robolectric.buildActivity(MainActivity::class.java).create(saved).start()

    private fun snapshotBytes() = runBlocking { requireNotNull(history.snapshot()).text.toByteArray(Charsets.UTF_8) }

    private fun shareFixture(id: String, modified: Long) = File(cache, "${DiagnosticLogFiles.SHARE_PREFIX}$id.txt").apply {
        parentFile!!.mkdirs()
        writeText("fixture")
        assertTrue(setLastModified(modified))
    }

    private fun launchPicker(activity: MainActivity): IntentForResult {
        activity.exportDiagnosticLogs()
        val launched = requireNotNull(shadowOf(activity).nextStartedActivityForResult)
        assertEquals(Intent.ACTION_CREATE_DOCUMENT, launched.intent.action)
        assertEquals("text/plain", launched.intent.type)
        assertEquals("connection-diagnostics.txt", launched.intent.getStringExtra(Intent.EXTRA_TITLE))
        return launched
    }

    private fun result(activity: MainActivity, launched: IntentForResult, uri: Uri) =
        deliverResult(activity, launched, Activity.RESULT_OK, uri)

    private fun deliverResult(activity: MainActivity, launched: IntentForResult, code: Int, uri: Uri = document) =
        shadowOf(activity).internalCallDispatchActivityResult(null, launched.requestCode, code, Intent().setData(uri))

    private fun waitFor(condition: () -> Boolean) {
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(5)
        do {
            shadowOf(Looper.getMainLooper()).idle()
            if (condition()) return
            Thread.yield()
        } while (System.nanoTime() < deadline)
        fail("Condition did not become true before timeout")
    }
}

// FileProvider expects Android's '/' separator. On a Windows test JVM, adapt only its
// canonical-path boundary check so the real provider and manifest can still be exercised.
@Implements(className = "androidx.core.content.FileProvider\$SimplePathStrategy", isInAndroidSdk = false,
    shadowPicker = WindowsFileProviderPathShadow.Picker::class)
class WindowsFileProviderPathShadow {
    @Implementation
    protected fun belongsToRoot(path: String, root: String): Boolean =
        path.trimEnd(File.separatorChar).startsWith(root.trimEnd(File.separatorChar) + File.separatorChar)

    class Picker : ShadowPicker<WindowsFileProviderPathShadow> {
        override fun pickShadowClass(): Class<out WindowsFileProviderPathShadow>? =
            if (File.separatorChar == '\\') WindowsFileProviderPathShadow::class.java else null
    }
}

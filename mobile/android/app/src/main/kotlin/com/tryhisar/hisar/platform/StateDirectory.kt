package com.tryhisar.hisar.platform

import android.content.Context
import android.system.Os
import java.io.File

/** Where Go keeps hisar.json and the offline cache: in the no-backup directory, readable by Hisar alone. */
object StateDirectory {
    fun prepare(context: Context): File {
        val directory = File(context.noBackupFilesDir, "Hisar")
        directory.mkdirs()
        Os.chmod(directory.path, "700".toInt(8))
        return directory
    }
}

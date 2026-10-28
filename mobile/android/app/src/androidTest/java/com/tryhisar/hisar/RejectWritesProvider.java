package com.tryhisar.hisar;

import android.database.Cursor;
import android.database.MatrixCursor;
import android.os.CancellationSignal;
import android.os.ParcelFileDescriptor;
import android.provider.DocumentsContract.Document;
import android.provider.DocumentsContract.Root;
import android.provider.DocumentsProvider;
import java.io.FileNotFoundException;

public final class RejectWritesProvider extends DocumentsProvider {
    private static final String[] ROOT_COLUMNS = {
        Root.COLUMN_ROOT_ID, Root.COLUMN_DOCUMENT_ID, Root.COLUMN_TITLE,
        Root.COLUMN_FLAGS, Root.COLUMN_MIME_TYPES, Root.COLUMN_AVAILABLE_BYTES
    };
    private static final String[] DOCUMENT_COLUMNS = {
        Document.COLUMN_DOCUMENT_ID, Document.COLUMN_DISPLAY_NAME, Document.COLUMN_MIME_TYPE,
        Document.COLUMN_FLAGS, Document.COLUMN_SIZE
    };

    @Override public boolean onCreate() { return true; }

    @Override public Cursor queryRoots(String[] projection) {
        MatrixCursor cursor = new MatrixCursor(projection == null ? ROOT_COLUMNS : projection);
        cursor.newRow().add(Root.COLUMN_ROOT_ID, "reject").add(Root.COLUMN_DOCUMENT_ID, "root")
            .add(Root.COLUMN_TITLE, "Reject writes").add(Root.COLUMN_FLAGS, Root.FLAG_SUPPORTS_CREATE)
            .add(Root.COLUMN_MIME_TYPES, "application/pdf").add(Root.COLUMN_AVAILABLE_BYTES, 1024 * 1024);
        return cursor;
    }

    @Override public Cursor queryDocument(String id, String[] projection) {
        MatrixCursor cursor = new MatrixCursor(projection == null ? DOCUMENT_COLUMNS : projection);
        boolean root = id.equals("root");
        cursor.newRow().add(Document.COLUMN_DOCUMENT_ID, id)
            .add(Document.COLUMN_DISPLAY_NAME, root ? "Reject writes" : id)
            .add(Document.COLUMN_MIME_TYPE, root ? Document.MIME_TYPE_DIR : "application/pdf")
            .add(Document.COLUMN_FLAGS, root ? Document.FLAG_DIR_SUPPORTS_CREATE : Document.FLAG_SUPPORTS_WRITE)
            .add(Document.COLUMN_SIZE, 0);
        return cursor;
    }

    @Override public Cursor queryChildDocuments(String parentId, String[] projection, String sortOrder) {
        return new MatrixCursor(projection == null ? DOCUMENT_COLUMNS : projection);
    }

    @Override public String createDocument(String parentId, String mimeType, String displayName) {
        return "unwritable.pdf";
    }

    @Override public ParcelFileDescriptor openDocument(String id, String mode, CancellationSignal signal) throws FileNotFoundException {
        throw new FileNotFoundException("Experiment rejects writes");
    }
}

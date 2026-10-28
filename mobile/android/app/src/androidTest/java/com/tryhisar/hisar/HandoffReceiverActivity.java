package com.tryhisar.hisar;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.widget.TextView;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;

public final class HandoffReceiverActivity extends Activity {
    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        Uri uri = getIntent().getParcelableExtra(Intent.EXTRA_STREAM);
        try (InputStream input = getContentResolver().openInputStream(uri)) {
            ByteArrayOutputStream output = new ByteArrayOutputStream();
            byte[] buffer = new byte[8192];
            int count;
            while ((count = input.read(buffer)) != -1) output.write(buffer, 0, count);
            byte[] bytes = output.toByteArray();
            if (!new String(bytes, 0, 4, StandardCharsets.US_ASCII).equals("%PDF")) {
                throw new IllegalStateException("The shared file is not a PDF");
            }
            TextView text = new TextView(this);
            text.setText("Received " + bytes.length + " PDF bytes");
            setContentView(text);
        } catch (IOException error) {
            throw new IllegalStateException("The receiver cannot read the shared PDF", error);
        }
    }
}

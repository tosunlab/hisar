package com.tryhisar.hisar.autofill;

import android.app.Activity;
import android.os.Bundle;
import android.text.Editable;
import android.text.InputType;
import android.text.TextWatcher;
import android.view.View;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;

/**
 * A native sign-in form in another package, for the Android app path (D10). Its report mirrors what was filled. Java for
 * the reason {@link AutofillTestPageActivity} gives.
 */
public class AutofillTestFormActivity extends Activity {
    public static final String EXTRA_CODE = "include-code";

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        TextView report = new TextView(this);
        report.setText("nothing typed");
        report.setContentDescription("report");
        EditText username = new EditText(this);
        username.setHint("Email");
        username.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_EMAIL_ADDRESS);
        username.setAutofillHints(View.AUTOFILL_HINT_USERNAME);
        EditText password = new EditText(this);
        password.setHint("Password");
        password.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD);
        EditText code = new EditText(this);
        code.setHint("Code");
        code.setInputType(InputType.TYPE_CLASS_NUMBER);
        code.setAutofillHints("2faAppOTPCode");
        boolean includeCode = getIntent().getBooleanExtra(EXTRA_CODE, false);
        TextWatcher watcher = new TextWatcher() {
            @Override
            public void beforeTextChanged(CharSequence text, int start, int count, int after) {}

            @Override
            public void onTextChanged(CharSequence text, int start, int before, int count) {}

            @Override
            public void afterTextChanged(Editable text) {
                report.setText("username=" + username.getText() + "; password=" + password.getText().length() + " characters" + (includeCode ? "; code=" + code.getText() : ""));
            }
        };
        username.addTextChangedListener(watcher);
        password.addTextChangedListener(watcher);
        code.addTextChangedListener(watcher);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setFitsSystemWindows(true);
        content.addView(username);
        content.addView(password);
        if (includeCode) content.addView(code);
        content.addView(report);
        setContentView(content);
    }
}

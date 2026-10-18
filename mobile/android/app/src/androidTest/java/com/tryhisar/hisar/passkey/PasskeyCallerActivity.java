package com.tryhisar.hisar.passkey;

import android.annotation.TargetApi;
import android.app.Activity;
import android.credentials.CreateCredentialException;
import android.credentials.CreateCredentialRequest;
import android.credentials.CreateCredentialResponse;
import android.credentials.CredentialManager;
import android.credentials.CredentialOption;
import android.credentials.GetCredentialException;
import android.credentials.GetCredentialRequest;
import android.credentials.GetCredentialResponse;
import android.os.Bundle;
import android.os.CancellationSignal;
import android.os.OutcomeReceiver;
import android.widget.FrameLayout;
import android.widget.TextView;

/**
 * A native app asking Credential Manager for passkeys, in the test APK's own package and process. It is Java on the
 * platform's API, because that process has neither the Kotlin library nor androidx.credentials, which only Hisar's APK
 * carries. The bundles are the ones androidx.credentials writes. The outcome shows as the text of "caller-result".
 */
@TargetApi(34)
public class PasskeyCallerActivity extends Activity {
    public static final String EXTRA_MODE = "mode";
    public static final String EXTRA_REQUEST_JSON = "requestJson";
    public static final String EXTRA_USER_NAME = "userName";

    private static final String TYPE_PUBLIC_KEY_CREDENTIAL = "androidx.credentials.TYPE_PUBLIC_KEY_CREDENTIAL";
    private static final String KEY_SUBTYPE = "androidx.credentials.BUNDLE_KEY_SUBTYPE";
    private static final String KEY_REQUEST_JSON = "androidx.credentials.BUNDLE_KEY_REQUEST_JSON";
    private static final String KEY_REQUEST_DISPLAY_INFO = "androidx.credentials.BUNDLE_KEY_REQUEST_DISPLAY_INFO";
    private static final String KEY_USER_ID = "androidx.credentials.BUNDLE_KEY_USER_ID";
    private static final String SUBTYPE_CREATE = "androidx.credentials.BUNDLE_VALUE_SUBTYPE_CREATE_PUBLIC_KEY_CREDENTIAL_REQUEST";
    private static final String SUBTYPE_GET = "androidx.credentials.BUNDLE_VALUE_SUBTYPE_GET_PUBLIC_KEY_CREDENTIAL_OPTION";
    private static final String KEY_REGISTRATION_RESPONSE_JSON = "androidx.credentials.BUNDLE_KEY_REGISTRATION_RESPONSE_JSON";
    private static final String KEY_AUTHENTICATION_RESPONSE_JSON = "androidx.credentials.BUNDLE_KEY_AUTHENTICATION_RESPONSE_JSON";

    private TextView result;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        result = new TextView(this);
        result.setContentDescription("caller-result");
        result.setText("waiting");
        FrameLayout layout = new FrameLayout(this);
        layout.setFitsSystemWindows(true);
        layout.addView(result);
        setContentView(layout);
        if (savedInstanceState != null) return;
        String requestJson = getIntent().getStringExtra(EXTRA_REQUEST_JSON);
        CredentialManager credentialManager = getSystemService(CredentialManager.class);
        if ("create".equals(getIntent().getStringExtra(EXTRA_MODE))) {
            Bundle data = buildBundle(SUBTYPE_CREATE, requestJson);
            Bundle displayInfo = new Bundle();
            displayInfo.putCharSequence(KEY_USER_ID, getIntent().getStringExtra(EXTRA_USER_NAME));
            data.putBundle(KEY_REQUEST_DISPLAY_INFO, displayInfo);
            CreateCredentialRequest request = new CreateCredentialRequest.Builder(TYPE_PUBLIC_KEY_CREDENTIAL, data, buildBundle(SUBTYPE_CREATE, requestJson))
                .setIsSystemProviderRequired(false)
                .setAlwaysSendAppInfoToProvider(true)
                .build();
            credentialManager.createCredential(this, request, new CancellationSignal(), getMainExecutor(), new OutcomeReceiver<CreateCredentialResponse, CreateCredentialException>() {
                @Override
                public void onResult(CreateCredentialResponse response) {
                    show("registered " + response.getData().getString(KEY_REGISTRATION_RESPONSE_JSON));
                }

                @Override
                public void onError(CreateCredentialException error) {
                    show("error " + error.getType() + " " + error.getMessage());
                }
            });
        } else {
            CredentialOption option = new CredentialOption.Builder(TYPE_PUBLIC_KEY_CREDENTIAL, buildBundle(SUBTYPE_GET, requestJson), buildBundle(SUBTYPE_GET, requestJson)).build();
            GetCredentialRequest request = new GetCredentialRequest.Builder(new Bundle()).addCredentialOption(option).build();
            credentialManager.getCredential(this, request, new CancellationSignal(), getMainExecutor(), new OutcomeReceiver<GetCredentialResponse, GetCredentialException>() {
                @Override
                public void onResult(GetCredentialResponse response) {
                    show("signed-in " + response.getCredential().getData().getString(KEY_AUTHENTICATION_RESPONSE_JSON));
                }

                @Override
                public void onError(GetCredentialException error) {
                    show("error " + error.getType() + " " + error.getMessage());
                }
            });
        }
    }

    private static Bundle buildBundle(String subtype, String requestJson) {
        Bundle bundle = new Bundle();
        bundle.putString(KEY_SUBTYPE, subtype);
        bundle.putString(KEY_REQUEST_JSON, requestJson);
        return bundle;
    }

    private void show(String text) {
        result.setText(text);
    }
}

package com.tryhisar.hisar.autofill;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import java.io.ByteArrayInputStream;
import java.nio.charset.StandardCharsets;

/**
 * A sign-in page the AutoFill service fills in the UI tests. It lives in the test APK, a package other than Hisar's, since
 * the service never fills Hisar itself; the debug build trusts that package's page addresses as a browser's. Both pages
 * are served from here at https://accounts.example.com, as a site would serve them: WebView 113 (API 34) reports no
 * page address for a page loaded with loadDataWithBaseURL. Signing in navigates to the welcome page, which is what
 * makes the system offer to save.
 *
 * <p>Java on framework classes only: this activity runs in the test APK's own process, whose dex has no Kotlin runtime.
 */
public class AutofillTestPageActivity extends Activity {
    public static final String EXTRA_PAGE = "page";

    private static final String REPORT = """
        <p id="result">nothing typed</p>
        <script>
          function report() {
            const fields = [...document.querySelectorAll('input')].map(input => input.name + '=' + (input.type === 'password' ? input.value.length + ' characters' : input.value));
            document.getElementById('result').textContent = fields.join('; ');
          }
          document.querySelectorAll('input').forEach(input => input.addEventListener('input', report));
          setInterval(report, 300);
        </script>""";

    public static final String LOGIN_PAGE = """
        <!doctype html><html><head><meta name="viewport" content="width=device-width"></head><body>
        <form action="welcome" method="get">
          <p><input id="username" name="username" autocomplete="username" placeholder="Email"></p>
          <p><input id="password" name="password" type="password" autocomplete="current-password" placeholder="Password"></p>
          <p><button type="submit">Sign in</button></p>
        </form>""" + REPORT + "</body></html>";

    public static final String ONE_TIME_CODE_PAGE = """
        <!doctype html><html><head><meta name="viewport" content="width=device-width"></head><body>
        <form action="javascript:void(0)">
          <p><input id="code" name="code" autocomplete="one-time-code" inputmode="numeric" placeholder="Code"></p>
        </form>""" + REPORT + "</body></html>";

    private static final String WELCOME_PAGE = "<!doctype html><html><body><p id=\"result\">signed in</p></body></html>";

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        String extra = getIntent().getStringExtra(EXTRA_PAGE);
        String page = extra != null ? extra : LOGIN_PAGE;
        WebView webView = new WebView(this);
        webView.getSettings().setJavaScriptEnabled(true);
        webView.setWebViewClient(new WebViewClient() {
            @Override
            public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                String path = request.getUrl().getPath();
                String html = "/welcome".equals(path) ? WELCOME_PAGE : "/login".equals(path) ? page : null;
                if (html == null) return null;
                return new WebResourceResponse("text/html", "utf-8", new ByteArrayInputStream(html.getBytes(StandardCharsets.UTF_8)));
            }
        });
        FrameLayout content = new FrameLayout(this);
        content.setFitsSystemWindows(true);
        content.addView(webView);
        setContentView(content);
        webView.loadUrl("https://accounts.example.com/login");
    }
}

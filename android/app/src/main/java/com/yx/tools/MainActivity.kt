package com.cf.probe

import android.annotation.SuppressLint
import android.app.Activity
import android.content.ContentValues
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.os.Environment
import android.provider.MediaStore
import android.webkit.JavascriptInterface
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
import androidlib.Androidlib
import java.io.File
import java.net.HttpURLConnection
import java.net.URL

class MainActivity : Activity() {

    private lateinit var webView: WebView

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        webView = WebView(this)
        setContentView(webView)
        webView.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true
            allowFileAccess = false
            allowContentAccess = false
        }
        webView.addJavascriptInterface(DownloadBridge(), "AndroidBridge")
        webView.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                val host = request.url.host ?: return false
                if (host == "127.0.0.1" || host == "localhost") return false
                // 界面里的外链（GitHub、博客、TG 群）交给系统浏览器打开
                return try {
                    startActivity(Intent(Intent.ACTION_VIEW, request.url))
                    true
                } catch (_: Exception) {
                    true
                }
            }
        }
        // 「下载」按钮走 /api/download，WebView 把它交给 DownloadListener
        webView.setDownloadListener { url, _, contentDisposition, _, _ ->
            downloadResult(url, contentDisposition)
        }
        startEngine()
    }

    private fun startEngine() {
        Thread {
            try {
                // 数据目录用应用私有目录：结果、配置、Token 都落在这里，随卸载清除
                val port = Androidlib.start(filesDir.absolutePath)
                runOnUiThread { webView.loadUrl("http://127.0.0.1:${port.toInt()}/") }
            } catch (e: Exception) {
                runOnUiThread {
                    Toast.makeText(this, "引擎启动失败: ${e.message}", Toast.LENGTH_LONG).show()
                }
            }
        }.start()
    }

    private fun downloadResult(url: String, contentDisposition: String?) {
        val name = fileName(url, contentDisposition)
        Thread {
            try {
                val conn = URL(url).openConnection() as HttpURLConnection
                conn.connectTimeout = 10000
                conn.readTimeout = 60000
                val data = conn.inputStream.use { it.readBytes() }
                conn.disconnect()
                val where = saveToDownloads(name, data)
                runOnUiThread {
                    Toast.makeText(
                        this,
                        if (where != null) "已保存 $name\n$where" else "保存失败",
                        Toast.LENGTH_LONG
                    ).show()
                }
            } catch (e: Exception) {
                runOnUiThread {
                    Toast.makeText(this, "下载失败: ${e.message}", Toast.LENGTH_LONG).show()
                }
            }
        }.start()
    }

    private inner class DownloadBridge {
        @JavascriptInterface
        fun saveTextFile(name: String, content: String) {
            if (name != "result-filtered.csv") {
                runOnUiThread {
                    Toast.makeText(this@MainActivity, "不支持的下载文件", Toast.LENGTH_SHORT).show()
                }
                return
            }
            Thread {
                val where = saveToDownloads(name, content.toByteArray(Charsets.UTF_8))
                runOnUiThread {
                    Toast.makeText(
                        this@MainActivity,
                        if (where != null) "已保存 $name\n$where" else "保存失败",
                        Toast.LENGTH_LONG
                    ).show()
                }
            }.start()
        }
    }

    private fun fileName(url: String, contentDisposition: String?): String {
        contentDisposition?.let {
            val m = Regex("filename\\s*=\\s*\"?([^\";]+)").find(it)
            if (m != null) return m.groupValues[1].trim()
        }
        return if (url.contains("kind=proxy")) "ips_ports.txt" else "result.csv"
    }

    private fun saveToDownloads(name: String, data: ByteArray): String? {
        return try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                // Android 10+ 走 MediaStore 存进系统「下载」，无需任何权限
                val values = ContentValues().apply {
                    put(MediaStore.Downloads.DISPLAY_NAME, name)
                    put(
                        MediaStore.Downloads.MIME_TYPE,
                        if (name.endsWith(".csv", ignoreCase = true)) "text/csv" else "text/plain"
                    )
                }
                val uri =
                    contentResolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values)
                        ?: return null
                contentResolver.openOutputStream(uri)?.use { it.write(data) } ?: return null
                "（系统下载目录）"
            } else {
                // Android 8/9：存应用外部专属目录，同样免权限
                val dir = getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS) ?: filesDir
                File(dir, name).writeBytes(data)
                dir.absolutePath
            }
        } catch (_: Exception) {
            null
        }
    }

    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        if (webView.canGoBack()) webView.goBack() else super.onBackPressed()
    }

    override fun onDestroy() {
        // 退出时关掉本地引擎；进程被系统回收时也会随进程一起结束
        Androidlib.stop()
        super.onDestroy()
    }
}

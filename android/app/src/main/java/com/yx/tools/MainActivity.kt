package com.cf.probe

import android.annotation.SuppressLint
import android.app.Activity
import android.content.ContentValues
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Environment
import android.provider.MediaStore
import android.webkit.JavascriptInterface
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
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
    private var fileChooserCallback: ValueCallback<Array<Uri>>? = null

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        webView = WebView(this)
        setContentView(webView)
        webView.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true
            allowFileAccess = false
            // 文件选择器返回的是 content:// URI，必须允许读取，
            // 否则选完文件后 JS 的 FileReader 读不到内容。
            allowContentAccess = true
        }
        webView.addJavascriptInterface(DownloadBridge(), "AndroidBridge")
        // 没有 WebChromeClient 时，网页里的 <input type="file"> 点不动，
        // 这里接管系统文件选择器，把选中的文件回传给网页。
        webView.webChromeClient = object : WebChromeClient() {
            override fun onShowFileChooser(
                view: WebView?,
                callback: ValueCallback<Array<Uri>>?,
                params: FileChooserParams?
            ): Boolean {
                fileChooserCallback?.onReceiveValue(null)
                fileChooserCallback = callback
                val intent = try {
                    params?.createIntent() ?: Intent(Intent.ACTION_GET_CONTENT).apply {
                        addCategory(Intent.CATEGORY_OPENABLE)
                        type = "*/*"
                    }
                } catch (_: Exception) {
                    callback?.onReceiveValue(null)
                    fileChooserCallback = null
                    return false
                }
                return try {
                    this@MainActivity.startActivityForResult(intent, FILE_CHOOSER_REQUEST)
                    true
                } catch (_: Exception) {
                    callback?.onReceiveValue(null)
                    fileChooserCallback = null
                    false
                }
            }
        }
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
                    Toast.makeText(this, "已保存 $name\n$where", Toast.LENGTH_LONG).show()
                }
            } catch (e: Exception) {
                runOnUiThread {
                    Toast.makeText(this, "下载失败: ${e.message}", Toast.LENGTH_LONG).show()
                }
            }
        }.start()
    }

    // 界面里能导出的文件：完整结果与筛选结果。
    // 安卓统一存成 .csv.txt，部分机型和文件管理器对 .csv 的识别不稳定。
    private val allowedDownloads = setOf(
        "result.csv.txt", "result-filtered.csv.txt",
        "result.csv", "result-filtered.csv"
    )

    private inner class DownloadBridge {
        @JavascriptInterface
        fun saveTextFile(name: String, content: String) {
            if (name !in allowedDownloads) {
                runOnUiThread {
                    Toast.makeText(this@MainActivity, "不支持的下载文件", Toast.LENGTH_SHORT).show()
                }
                return
            }
            Thread {
                try {
                    val where = saveToDownloads(name, content.toByteArray(Charsets.UTF_8))
                    runOnUiThread {
                        Toast.makeText(
                            this@MainActivity,
                            "已保存 $name\n$where",
                            Toast.LENGTH_LONG
                        ).show()
                    }
                } catch (e: Exception) {
                    runOnUiThread {
                        Toast.makeText(
                            this@MainActivity,
                            "保存失败: ${e.message}",
                            Toast.LENGTH_LONG
                        ).show()
                    }
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

    private fun saveToDownloads(name: String, data: ByteArray): String {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            // Android 10+ 走 MediaStore 存进系统「下载」，无需任何权限。
            // 按官方推荐的 IS_PENDING 流程写，避免文件管理器只看到空文件。
            val values = ContentValues().apply {
                put(MediaStore.Downloads.DISPLAY_NAME, name)
                put(
                    MediaStore.Downloads.MIME_TYPE,
                    if (name.endsWith(".csv", ignoreCase = true)) "text/csv" else "text/plain"
                )
                put(MediaStore.Downloads.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
                put(MediaStore.Downloads.IS_PENDING, 1)
            }
            val uri = contentResolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values)
                ?: throw IllegalStateException("无法创建下载文件")
            try {
                contentResolver.openOutputStream(uri)?.use { it.write(data) }
                    ?: throw IllegalStateException("无法写入下载文件")
                values.clear()
                values.put(MediaStore.Downloads.IS_PENDING, 0)
                contentResolver.update(uri, values, null, null)
            } catch (e: Exception) {
                contentResolver.delete(uri, null, null)
                throw e
            }
            return "（系统下载目录）"
        }
        // Android 8/9：存应用外部专属目录，同样免权限
        val dir = getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS) ?: filesDir
        File(dir, name).writeBytes(data)
        return dir.absolutePath
    }

    // 系统文件选择器返回后，把选中的文件交给 WebView
    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        if (requestCode == FILE_CHOOSER_REQUEST) {
            val callback = fileChooserCallback
            fileChooserCallback = null
            callback?.onReceiveValue(
                if (resultCode == RESULT_OK && data != null)
                    WebChromeClient.FileChooserParams.parseResult(resultCode, data)
                else null
            )
            return
        }
        super.onActivityResult(requestCode, resultCode, data)
    }

    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        if (webView.canGoBack()) webView.goBack() else super.onBackPressed()
    }

    override fun onDestroy() {
        // 退出时关掉本地引擎；进程被系统回收时也会随进程一起结束
        fileChooserCallback?.onReceiveValue(null)
        fileChooserCallback = null
        Androidlib.stop()
        super.onDestroy()
    }

    private companion object {
        const val FILE_CHOOSER_REQUEST = 1001
    }
}

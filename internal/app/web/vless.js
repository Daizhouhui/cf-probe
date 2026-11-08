'use strict';

// 隐藏的 VLESS 订阅功能：只有拿到 NodeWeb 令牌并校验成功后才出现入口。
// 本文件依赖 app.js 暴露的全局函数 api() / toast()。
(function () {
  const $ = (s) => document.querySelector(s);
  const api = window.api;
  const toast = window.toast;

  function isLocalPage() {
    const h = location.hostname;
    return h === '127.0.0.1' || h === 'localhost' || h === '::1';
  }

  let devClicks = 0;
  let devTimer = 0;

  async function refreshStatus() {
    if (!isLocalPage()) return null;
    try {
      const st = await api('/api/vless/status');
      const on = !!(st && st.enabled);
      $('#btnVless').classList.toggle('hidden', !on);
      return st;
    } catch (_) {
      return null;
    }
  }

  // 连点「上报配置」标题 7 次才露出开发者选项，平时没有任何提示。
  function bindDevGesture() {
    const title = document.querySelector('#mask .modal h3');
    if (!title) return;
    title.addEventListener('click', () => {
      if (!isLocalPage()) return;
      devClicks++;
      clearTimeout(devTimer);
      devTimer = setTimeout(() => { devClicks = 0; }, 3000);
      if (devClicks >= 7) {
        devClicks = 0;
        $('#vlessDev').classList.remove('hidden');
      }
    });
  }

  function fillSelect(id, values, current) {
    const sel = $(id);
    if (!sel) return;
    sel.innerHTML = '';
    const seen = {};
    (values || []).forEach(v => {
      if (!v || seen[v]) return;
      seen[v] = true;
      const opt = document.createElement('option');
      opt.value = v;
      opt.textContent = v;
      sel.appendChild(opt);
    });
    if (current && !seen[current]) {
      const opt = document.createElement('option');
      opt.value = current;
      opt.textContent = current;
      sel.appendChild(opt);
    }
    if (sel.options.length) {
      sel.value = current || sel.options[0].value;
    }
  }

  function valueOr(a, b) {
    return (a != null && a !== '') ? a : (b || '');
  }

  function fillForm(st) {
    const d = st.defaults || {};
    const p = st.presets || {};
    const meta = [st.config_repo, st.config_branch, st.config_path].filter(Boolean).join('/');
    let line = meta || '配置已加载';
    if (st.fetched_at) line += ' · 更新于 ' + new Date(st.fetched_at).toLocaleString();
    if (st.warning) line += ' · ' + st.warning;
    $('#vlessMeta').textContent = line;

    fillSelect('#vlessUUID', [d.uuid].concat(p.uuid || []), valueOr(st.uuid, d.uuid));
    fillSelect('#vlessHost', [d.host].concat(p.host || []), valueOr(st.host, d.host));
    fillSelect('#vlessSNI', [d.sni].concat(p.sni || []), valueOr(st.sni, d.sni));
    fillSelect('#vlessPath', [d.path].concat(p.path || []), valueOr(st.path, d.path));
    fillSelect('#vlessFP', [d.fp].concat(p.fp || []), valueOr(st.fp, d.fp));
    $('#vlessECH').checked = !!st.ech;
    $('#vlessUseConfig').checked = !!st.use_config_for_upload;
    $('#vlessUploadRepo').value = st.upload_repo || '';
    $('#vlessUploadToken').value = '';
    $('#vlessUploadToken').placeholder = st.has_upload_token ? '留空沿用已保存的' : '必填';
    $('#vlessFile').value = st.file || 'vlesssub';
    $('#vlessCount').value = st.count ? st.count : '';
    $('#vlessMerge').checked = st.merge !== false;

    if (st.subscription_url) {
      renderResult('上次订阅：', st.subscription_url);
    } else {
      renderResult('', '');
    }
    syncUseConfig();
  }

  function syncUseConfig() {
    const on = $('#vlessUseConfig').checked;
    $('#vlessUploadRepo').disabled = on;
    $('#vlessUploadToken').disabled = on;
  }

  function renderResult(text, url) {
    const box = $('#vlessResult');
    box.textContent = '';
    if (text) {
      text.split('\n').forEach((line, i) => {
        if (i) box.appendChild(document.createElement('br'));
        box.appendChild(document.createTextNode(line));
      });
    }
    if (url) {
      if (box.childNodes.length) box.appendChild(document.createElement('br'));
      const a = document.createElement('a');
      a.href = url;
      a.target = '_blank';
      a.rel = 'noreferrer';
      a.textContent = url;
      box.appendChild(a);
      const copy = document.createElement('button');
      copy.type = 'button';
      copy.className = 'btn ghost sm';
      copy.textContent = '复制';
      copy.style.marginLeft = '6px';
      copy.onclick = () => navigator.clipboard.writeText(url).then(
        () => toast('已复制订阅链接', 'ok'),
        () => toast('复制失败', 'err')
      );
      box.appendChild(copy);

      // 安卓上尝试直接唤起 v2rayNG 导入订阅；桌面端不显示。
      if (/Android/i.test(navigator.userAgent) || window.AndroidBridge) {
        const imp = document.createElement('a');
        imp.className = 'btn ghost sm';
        imp.textContent = '导入 v2rayNG';
        imp.style.marginLeft = '6px';
        imp.href = 'v2rayng://install-sub?url=' + encodeURIComponent(url);
        box.appendChild(imp);
      }
    }
  }

  async function open(st) {
    if (!isLocalPage()) return;
    try {
      const s = st || await api('/api/vless/status');
      if (!s || !s.enabled) {
        toast('VLESS 功能未启用', 'err');
        return;
      }
      fillForm(s);
      $('#vlessMask').classList.remove('hidden');
    } catch (e) {
      toast(e.message, 'err');
    }
  }

  async function refreshConfig() {
    const btn = $('#vlessRefresh');
    btn.disabled = true;
    try {
      const st = await api('/api/vless/config/refresh', { method: 'POST' });
      fillForm(st);
      toast('配置已刷新', 'ok');
    } catch (e) {
      toast(e.message, 'err');
    } finally {
      btn.disabled = false;
    }
  }

  async function enable() {
    const token = $('#vlessEnableToken').value.trim();
    if (!token) {
      toast('请填写 NodeWeb 令牌', 'err');
      return;
    }
    try {
      const st = await api('/api/vless/enable', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          token,
          repo: $('#vlessCfgRepo').value.trim(),
          branch: $('#vlessCfgBranch').value.trim(),
          path: $('#vlessCfgPath').value.trim(),
        }),
      });
      $('#vlessEnableToken').value = '';
      toast('VLESS 功能已启用', 'ok');
      await refreshStatus();
      await open(st);
    } catch (e) {
      toast(e.message, 'err');
    }
  }

  async function upload() {
    const body = {
      count: $('#vlessCount').value.trim() === '' ? 0 : +$('#vlessCount').value,
      file: $('#vlessFile').value.trim() || 'vlesssub',
      merge: $('#vlessMerge').checked,
      ech: $('#vlessECH').checked,
      uuid: $('#vlessUUID').value.trim(),
      host: $('#vlessHost').value.trim(),
      sni: $('#vlessSNI').value.trim(),
      path: $('#vlessPath').value.trim(),
      fp: $('#vlessFP').value.trim(),
      use_config_for_upload: $('#vlessUseConfig').checked,
    };
    if (!body.use_config_for_upload) {
      body.upload_repo = $('#vlessUploadRepo').value.trim();
      body.upload_token = $('#vlessUploadToken').value.trim();
    }

    const btn = $('#vlessRun');
    btn.disabled = true;
    renderResult('正在生成并上报…', '');
    try {
      const r = await api('/api/vless/upload', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      let msg = `已上报 ${r.count} 个节点到 ${r.file}`;
      if (r.merged) msg += '（已合并去重）';
      if (r.warning) msg += '\n提示：' + r.warning;
      if (r.subscription_error) msg += '\n订阅链接获取失败：' + r.subscription_error;
      renderResult(msg, r.url || '');
      $('#vlessUploadToken').value = '';
      toast('已上报 ' + r.count + ' 个节点', 'ok');
    } catch (e) {
      renderResult('上报失败：' + e.message, '');
      toast(e.message, 'err');
    } finally {
      btn.disabled = false;
    }
  }

  async function init() {
    if (!isLocalPage()) return;
    bindDevGesture();
    $('#btnVless').addEventListener('click', () => open());
    $('#btnVlessEnable').addEventListener('click', enable);
    $('#vlessClose').addEventListener('click', () => $('#vlessMask').classList.add('hidden'));
    $('#vlessMask').addEventListener('click', e => {
      if (e.target === $('#vlessMask')) $('#vlessMask').classList.add('hidden');
    });
    $('#vlessRefresh').addEventListener('click', refreshConfig);
    $('#vlessRun').addEventListener('click', upload);
    $('#vlessUseConfig').addEventListener('change', syncUseConfig);
    await refreshStatus();
  }

  window.VlessUI = { refreshStatus, open };

  document.addEventListener('DOMContentLoaded', init);
})();

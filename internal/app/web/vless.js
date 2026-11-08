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
      if (window.VlessUI && window.VlessUI.open) window.VlessUI.open(st);
    } catch (e) {
      toast(e.message, 'err');
    }
  }

  window.VlessUI = { refreshStatus, open: () => {} };

  async function init() {
    if (!isLocalPage()) return;
    bindDevGesture();
    $('#btnVless').addEventListener('click', () => window.VlessUI.open());
    $('#btnVlessEnable').addEventListener('click', enable);
    await refreshStatus();
  }

  document.addEventListener('DOMContentLoaded', init);
})();

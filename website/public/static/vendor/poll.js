(function () {
  var script = document.currentScript;
  var orderNo = script.dataset.order;
  var returnUrl = script.dataset.return || '';
  var successUrl = script.dataset.success || '';
  var expiredUrl = script.dataset.expired || '';
  var completed = false;

  function setStatus(text) {
    var status = document.getElementById('status');
    if (status) status.textContent = text;
  }

  function showSuccess() {
    var modal = document.getElementById('success-modal');
    if (modal) {
      modal.classList.remove('hidden');
      modal.classList.add('flex');
    }
  }

  function closeExpiredQr(targetUrl) {
    var qrPanel = document.getElementById('qr-panel');
    if (qrPanel) qrPanel.classList.add('hidden');
    setStatus('订单已超时，正在关闭付款二维码...');
    setTimeout(function () {
      window.location.replace(targetUrl);
    }, 300);
  }

  function tick() {
    if (completed) return;
    fetch('/pay/status/' + encodeURIComponent(orderNo))
      .then(function (res) { return res.json(); })
      .then(function (data) {
        if (data.expired && !completed) {
          completed = true;
          closeExpiredQr(data.expired_url || expiredUrl || returnUrl || '/');
          return;
        }
        if (!data.paid || completed) return;
        completed = true;
        var targetUrl = successUrl || data.success_url || returnUrl || '/';
        setStatus('支付成功，正在进入支付结果页...');
        showSuccess();
        setTimeout(function () {
          window.location.href = targetUrl;
        }, 900);
      })
      .catch(function () {});
  }

  tick();
  setInterval(tick, 3000);
})();

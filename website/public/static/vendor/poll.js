(function () {
  var script = document.currentScript;
  var orderNo = script.dataset.order;
  var returnUrl = script.dataset.return;
  function tick() {
    fetch('/pay/status/' + encodeURIComponent(orderNo))
      .then(function (res) { return res.json(); })
      .then(function (data) {
        if (data.paid) {
          document.getElementById('status').textContent = '支付成功，正在跳转...';
          if (returnUrl) window.location.href = returnUrl;
        }
      })
      .catch(function () {});
  }
  setInterval(tick, 3000);
})();

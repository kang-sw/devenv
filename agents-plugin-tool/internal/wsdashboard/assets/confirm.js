(function () {
  var pending = null;

  // Escape belongs to the top dialog before the dashboard's shortcuts see it.
  window.addEventListener("keydown", function (e) {
    if (!pending || e.key !== "Escape") return;
    e.preventDefault();
    e.stopImmediatePropagation();
    if (pending.open) pending.close();
  }, true);

  window.wsConfirm = async function (message, { accept, cancel, opener = document.activeElement } = {}) {
    if (pending) return false;
    var dialog = document.createElement("dialog");
    if (typeof dialog.showModal !== "function") return false;
    dialog.className = "rdeldlg ws-confirm";
    dialog.setAttribute("aria-labelledby", "ws-confirm-message");

    var title = dialog.appendChild(document.createElement("h2"));
    title.id = "ws-confirm-message";
    title.className = "rdhd";
    title.textContent = message;

    var actions = dialog.appendChild(document.createElement("div"));
    actions.className = "rdrow";
    var no = actions.appendChild(document.createElement("button"));
    no.type = "button";
    no.className = "pbtn";
    no.autofocus = true;
    no.textContent = cancel ?? wsT("js.confirm.cancel");
    var yes = actions.appendChild(document.createElement("button"));
    yes.type = "button";
    yes.className = "pbtn danger";
    yes.textContent = accept ?? wsT("js.confirm.accept");

    return new Promise(function (resolve) {
      var accepted = false;
      function finish() {
        dialog.remove();
        if (opener && opener.isConnected) opener.focus({ preventScroll: true });
        pending = null;
        resolve(accepted);
      }
      // Keep the underlying view's outside-click handlers from dismissing it.
      dialog.addEventListener("click", function (e) { e.stopPropagation(); });
      dialog.addEventListener("cancel", function (e) { e.preventDefault(); if (dialog.open) dialog.close(); });
      no.addEventListener("click", function () { if (dialog.open) dialog.close(); });
      yes.addEventListener("click", function () {
        if (!dialog.open) return;
        accepted = true;
        dialog.close();
      });
      dialog.addEventListener("close", finish, { once: true });
      pending = dialog;
      document.body.appendChild(dialog);
      try { dialog.showModal(); }
      catch (e) { finish(); return; }
      no.focus({ preventScroll: true });
    });
  };
}());

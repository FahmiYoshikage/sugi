function copyInstallCmd() {
  const cmd = "curl -fsSL https://raw.githubusercontent.com/FahmiYoshikage/sugi/master/install.sh | sh";
  navigator.clipboard.writeText(cmd).then(() => {
    const btn = document.getElementById("copy-btn");
    if (btn) {
      const orig = btn.innerText;
      btn.innerText = "Copied!";
      setTimeout(() => { btn.innerText = orig; }, 2000);
    }
  });
}

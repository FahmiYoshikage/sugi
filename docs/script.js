const installCommands = {
  curl: {
    plain: "curl -fsSL https://raw.githubusercontent.com/FahmiYoshikage/sugi/master/install.sh | sh",
    html: '<span class="hl-bin">curl</span> <span class="hl-flag">-fsSL</span> <span class="hl-url">https://raw.githubusercontent.com/FahmiYoshikage/sugi/master/install.sh</span> <span class="hl-op">|</span> <span class="hl-bin">sh</span>'
  },
  go: {
    plain: "go install github.com/FahmiYoshikage/sugi/cmd/sugi@latest",
    html: '<span class="hl-bin">go</span> <span class="hl-cmd">install</span> <span class="hl-url">github.com/FahmiYoshikage/sugi/cmd/sugi@latest</span>'
  },
  binary: {
    plain: "curl -LO https://github.com/FahmiYoshikage/sugi/releases/latest/download/sugi-linux-amd64 && chmod +x sugi-linux-amd64",
    html: '<span class="hl-bin">curl</span> <span class="hl-flag">-LO</span> <span class="hl-url">https://github.com/FahmiYoshikage/sugi/releases/latest/download/sugi-linux-amd64</span> <span class="hl-op">&amp;&amp;</span> <span class="hl-bin">chmod</span> <span class="hl-flag">+x</span> <span class="hl-param">sugi-linux-amd64</span>'
  }
};

let currentTab = 'curl';

function setInstallTab(tabKey) {
  if (!installCommands[tabKey]) return;
  currentTab = tabKey;

  document.querySelectorAll('.t-tab').forEach(btn => {
    btn.classList.remove('active');
  });
  const activeBtn = document.querySelector(`.t-tab[data-tab="${tabKey}"]`);
  if (activeBtn) activeBtn.classList.add('active');

  const contentEl = document.getElementById('terminal-content');
  if (contentEl) {
    contentEl.innerHTML = installCommands[tabKey].html;
  }
}

function copyCurrentCmd() {
  const item = installCommands[currentTab] || installCommands.curl;
  navigator.clipboard.writeText(item.plain).then(() => {
    const btn = document.getElementById('copy-btn');
    const icon = document.getElementById('copy-icon');
    const text = document.getElementById('copy-text');
    if (btn && icon && text) {
      btn.classList.add('copied');
      text.innerText = 'Copied!';
      icon.innerHTML = '<polyline points="20 6 9 17 4 12"></polyline>';
      setTimeout(() => {
        btn.classList.remove('copied');
        text.innerText = 'Copy';
        icon.innerHTML = '<rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>';
      }, 2000);
    }
  });
}

// Fallback for legacy calls
function copyInstallCmd() {
  copyCurrentCmd();
}

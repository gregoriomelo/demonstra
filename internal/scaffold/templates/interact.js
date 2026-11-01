export async function enableVisualInteractions(page) {
  await page.addInitScript(() => {
    function injectStyles() {
      if (document.getElementById('__demonstra_styles')) return;
      const s = document.createElement('style');
      s.id = '__demonstra_styles';
      s.textContent = `
        .__demonstra_highlight {
          outline: 3px solid #ef4444 !important;
          outline-offset: 3px !important;
          box-shadow: 0 0 18px rgba(239, 68, 68, 0.8) !important;
          transition: outline 0.15s ease-in-out, box-shadow 0.15s ease-in-out !important;
        }

        .__demonstra_cursor {
          position: fixed !important;
          width: 22px !important;
          height: 22px !important;
          border-radius: 50% !important;
          background: #ef4444 !important;
          border: 2.5px solid #ffffff !important;
          box-shadow: 0 2px 10px rgba(0, 0, 0, 0.45) !important;
          pointer-events: none !important;
          z-index: 2147483647 !important;
          transform: translate(-50%, -50%);
          display: none;
        }

        .__demonstra_radar {
          position: fixed !important;
          pointer-events: none !important;
          z-index: 2147483646 !important;
          transform: translate(-50%, -50%);
        }

        .__demonstra_radar_ring {
          position: absolute !important;
          top: 50% !important;
          left: 50% !important;
          border-radius: 50% !important;
          border: 2.5px solid #ef4444 !important;
          box-shadow: 0 0 12px rgba(239, 68, 68, 0.7) !important;
          transform: translate(-50%, -50%) scale(0.2);
          opacity: 1;
        }

        .__demonstra_radar_ring_1 {
          width: 48px !important;
          height: 48px !important;
          animation: __demonstra_radar_anim 0.65s cubic-bezier(0.1, 0.8, 0.2, 1) forwards;
        }

        .__demonstra_radar_ring_2 {
          width: 66px !important;
          height: 66px !important;
          animation: __demonstra_radar_anim 0.65s cubic-bezier(0.1, 0.8, 0.2, 1) 0.12s forwards;
        }

        .__demonstra_radar_ring_3 {
          width: 84px !important;
          height: 84px !important;
          animation: __demonstra_radar_anim 0.65s cubic-bezier(0.1, 0.8, 0.2, 1) 0.24s forwards;
        }

        @keyframes __demonstra_radar_anim {
          0% { transform: translate(-50%, -50%) scale(0.2); opacity: 0.95; }
          100% { transform: translate(-50%, -50%) scale(1.6); opacity: 0; }
        }

        .__demonstra_hud {
          position: fixed !important;
          bottom: 24px !important;
          left: 50% !important;
          transform: translateX(-50%) translateY(0) !important;
          background: rgba(15, 23, 42, 0.92) !important;
          backdrop-filter: blur(8px) !important;
          -webkit-backdrop-filter: blur(8px) !important;
          border: 1px solid rgba(255, 255, 255, 0.2) !important;
          border-radius: 9999px !important;
          padding: 8px 18px !important;
          color: #ffffff !important;
          font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif !important;
          font-size: 13px !important;
          font-weight: 500 !important;
          box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45) !important;
          pointer-events: none !important;
          z-index: 2147483647 !important;
          display: flex !important;
          align-items: center !important;
          gap: 8px !important;
          opacity: 0;
          transition: opacity 0.2s ease, transform 0.2s ease !important;
        }

        .__demonstra_hud.visible {
          opacity: 1 !important;
          transform: translateX(-50%) translateY(-6px) !important;
        }

        .__demonstra_hud kbd {
          background: #334155 !important;
          color: #f8fafc !important;
          border: 1px solid #64748b !important;
          border-radius: 4px !important;
          padding: 2px 7px !important;
          font-size: 11px !important;
          font-family: monospace !important;
          font-weight: 600 !important;
        }
      `;
      const target = document.head || document.body || document.documentElement;
      if (target) {
        target.appendChild(s);
      }
    }

    let cursor = null;
    let hud = null;
    let hudTimer = null;

    function getCursor() {
      injectStyles();
      if ((!cursor || !document.contains(cursor)) && (document.body || document.documentElement)) {
        cursor = document.createElement('div');
        cursor.className = '__demonstra_cursor';
        (document.body || document.documentElement).appendChild(cursor);
      }
      return cursor;
    }

    function getHUD() {
      injectStyles();
      if ((!hud || !document.contains(hud)) && (document.body || document.documentElement)) {
        hud = document.createElement('div');
        hud.className = '__demonstra_hud';
        (document.body || document.documentElement).appendChild(hud);
      }
      return hud;
    }

    function escapeHTML(str) {
      return String(str).replace(/[&<>"']/g, c => ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
      }[c]));
    }

    function showHUD(html, duration = 800) {
      const h = getHUD();
      if (!h) return;
      h.innerHTML = html;
      h.classList.add('visible');
      if (hudTimer) clearTimeout(hudTimer);
      hudTimer = setTimeout(() => {
        h.classList.remove('visible');
      }, duration);
    }

    function onPointer(e) {
      const c = getCursor();
      if (c) {
        c.style.display = 'block';
        c.style.left = e.clientX + 'px';
        c.style.top = e.clientY + 'px';
      }
    }

    function onClick(e) {
      injectStyles();
      onPointer(e);

      const targetParent = document.body || document.documentElement;
      if (targetParent) {
        const radar = document.createElement('div');
        radar.className = '__demonstra_radar';
        radar.style.left = e.clientX + 'px';
        radar.style.top = e.clientY + 'px';

        const ring1 = document.createElement('div');
        ring1.className = '__demonstra_radar_ring __demonstra_radar_ring_1';
        const ring2 = document.createElement('div');
        ring2.className = '__demonstra_radar_ring __demonstra_radar_ring_2';
        const ring3 = document.createElement('div');
        ring3.className = '__demonstra_radar_ring __demonstra_radar_ring_3';

        radar.appendChild(ring1);
        radar.appendChild(ring2);
        radar.appendChild(ring3);
        targetParent.appendChild(radar);
        setTimeout(() => radar.remove(), 950);
      }

      let el = e.target;
      while (el && el !== document.body && el !== document.documentElement) {
        if (el.tagName === 'BUTTON' || el.tagName === 'A' || el.tagName === 'INPUT' || el.getAttribute('role') === 'button' || el.classList.contains('toggle')) {
          break;
        }
        el = el.parentElement;
      }
      if (!el || el === document.body || el === document.documentElement) el = e.target;

      if (el && el !== document.body && el !== document.documentElement) {
        el.classList.add('__demonstra_highlight');
        setTimeout(() => el.classList.remove('__demonstra_highlight'), 750);

        let label = (el.innerText || el.getAttribute('aria-label') || el.placeholder || el.value || '').trim();
        if (!label) {
          const itemParent = el.closest('[id^="item_"]') || el.closest('li');
          if (itemParent) {
            const itemText = itemParent.querySelector('p, label')?.innerText?.trim();
            if (itemText) {
              label = `Toggle "${itemText}"`;
            }
          }
        }
        if (!label) {
          label = el.getAttribute('title') || el.tagName;
        }

        const shortLabel = label.length > 25 ? label.slice(0, 25) + '…' : label;
        showHUD(`<span>🖱️ Click</span> <strong style="color: #fca5a5;">${escapeHTML(shortLabel)}</strong>`, 750);
      }
    }

    function onKeyDown(e) {
      if (e.key === 'Enter') {
        showHUD(`<kbd>↵ Enter</kbd> <span>Submit</span>`, 800);
      } else if (e.key === 'Tab') {
        showHUD(`<kbd>⇥ Tab</kbd> <span>Next field</span>`, 700);
      } else if (e.key === 'Escape') {
        showHUD(`<kbd>Esc</kbd> <span>Cancel</span>`, 700);
      } else if (e.key === 'Backspace' && e.target && e.target.value === '') {
        showHUD(`<kbd>⌫ Backspace</kbd> <span>Clear</span>`, 700);
      }
    }

    let typeTimer = null;
    function onInput(e) {
      const val = e.target?.value;
      if (!val) return;
      if (typeTimer) clearTimeout(typeTimer);
      typeTimer = setTimeout(() => {
        const preview = val.length > 24 ? val.slice(0, 24) + '…' : val;
        showHUD(`<span>⌨️ Type</span> <strong style="color: #93c5fd;">"${escapeHTML(preview)}"</strong>`, 700);
      }, 80);
    }

    window.addEventListener('DOMContentLoaded', injectStyles);
    window.addEventListener('turbo:load', injectStyles);
    window.addEventListener('pointermove', onPointer, true);
    window.addEventListener('pointerdown', onClick, true);
    window.addEventListener('keydown', onKeyDown, true);
    window.addEventListener('input', onInput, true);
  });
}

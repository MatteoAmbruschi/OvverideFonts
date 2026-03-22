// Service worker: polls the Go app every second and notifies all tabs on change.
const API = "http://localhost:59842/font";

let lastFont = "";
let lastActive = false;
let lastFontUrl = "";

async function sync() {
  try {
    const r = await fetch(API);
    const { font, active, fontUrl = "" } = await r.json();
    if (font !== lastFont || active !== lastActive || fontUrl !== lastFontUrl) {
      lastFont = font;
      lastActive = active;
      lastFontUrl = fontUrl;
      const tabs = await chrome.tabs.query({});
      for (const tab of tabs) {
        if (tab.id) {
          chrome.tabs
            .sendMessage(tab.id, { font, active, fontUrl })
            .catch(() => {});
        }
      }
    }
  } catch (_) {}
}

setInterval(sync, 1000);
sync();

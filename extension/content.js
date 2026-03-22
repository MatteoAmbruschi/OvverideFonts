// Content script: injects/removes <style> tags that override all font-families.
// For bundled fonts (non-system), also injects @font-face pointing to localhost.
const FACE_ID = "fo-font-face";
const STYLE_ID = "fo-font-override";
const API = "http://localhost:59842/font";

function apply(font, fontUrl) {
  const safe = font.replace(/'/g, "\\'");

  // @font-face injection for bundled fonts served from localhost.
  if (fontUrl) {
    let face = document.getElementById(FACE_ID);
    if (!face) {
      face = document.createElement("style");
      face.id = FACE_ID;
      (document.head || document.documentElement).prepend(face);
    }
    face.textContent = `@font-face{font-family:'${safe}';src:url('${fontUrl}') format('opentype');}`;
  }

  // Override all elements.
  let s = document.getElementById(STYLE_ID);
  if (!s) {
    s = document.createElement("style");
    s.id = STYLE_ID;
    (document.head || document.documentElement).prepend(s);
  }
  s.textContent = `*,*::before,*::after{font-family:'${safe}' !important}`;
}

function reset() {
  document.getElementById(STYLE_ID)?.remove();
  document.getElementById(FACE_ID)?.remove();
}

// On page load: fetch current state immediately (no polling delay).
fetch(API)
  .then((r) => r.json())
  .then(({ font, active, fontUrl = "" }) => {
    if (active) apply(font, fontUrl);
  })
  .catch(() => {});

// Listen for real-time updates from the background service worker.
chrome.runtime.onMessage.addListener(({ font, active, fontUrl = "" }) => {
  active ? apply(font, fontUrl) : reset();
});

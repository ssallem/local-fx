import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";

const container = document.getElementById("root");
if (!container) {
  throw new Error("root element missing");
}

// tab.html's <title> is parsed before any script runs, so `__MSG_*__` tokens
// can't be resolved there — Chrome only substitutes those inside manifest
// fields. Instead, patch the document title at mount time using the live
// i18n locale so the browser tab label matches the localized app name.
document.title = chrome.i18n.getMessage("appName") || "Local Explorer";

createRoot(container).render(
  <StrictMode>
    <App />
  </StrictMode>
);

// app.js — entry point. Importing each module runs its setup (listeners attach on
// load; type="module" scripts are deferred, so the DOM is ready). Then boot the
// connection — unless we're showing the first-run wizard on the insecure setup
// origin, where there's nothing to control yet.
import { KEYS } from "./state.js";
import "./ui.js";
import { connect } from "./connection.js";
import "./trackpad.js";
import "./controls.js";
import { openSettings } from "./settings.js";
import "./pwa.js";
import { inSetup } from "./setup.js";

if (!inSetup) connect();

// First run from a file:// open with no saved host → nudge the user into Ajustes.
if (!localStorage.getItem(KEYS.STORAGE_KEY) && location.protocol === "file:") {
  setTimeout(openSettings, 400);
}

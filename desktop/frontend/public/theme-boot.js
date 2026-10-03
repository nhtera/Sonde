// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The theme before the first paint, before the settings load: the app's
// server writes the theme settings on <html> (data-theme-pref, -day and
// -night). A file, not an inline script: the page's CSP allows only
// scripts from the app.
(function () {
  var d = document.documentElement.dataset;
  var light = !!(window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches);
  var pref = d.themePref || "system";
  d.theme = pref !== "system" ? pref : light ? d.themeDay || "light" : d.themeNight || "dark";
})();

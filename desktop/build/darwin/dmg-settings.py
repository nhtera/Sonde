# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# dmgbuild settings for the release disk image: the app beside an
# Applications link on a background that says to drag one onto the
# other. desktop/scripts/make-dmg.sh passes the paths as -D defines.

app = defines["app"]  # noqa: F821 (dmgbuild provides defines)
here = defines["build"]  # noqa: F821

format = "UDZO"
files = [app]
symlinks = {"Applications": "/Applications"}
icon = here + "/icons.icns"
background = here + "/dmg-background.tiff"

window_rect = ((200, 120), (540, 380))
default_view = "icon-view"
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
icon_size = 128
text_size = 13
icon_locations = {"Sonde.app": (135, 175), "Applications": (405, 175)}

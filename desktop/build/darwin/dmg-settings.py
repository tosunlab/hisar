# dmgbuild settings for scripts/package-desktop-dmg.sh: Hisar.app on the left, Applications on the right.
import os.path

application = defines["application"]
application_name = os.path.basename(application)

format = "UDZO"
filesystem = "HFS+"
files = [application]
symlinks = {"Applications": "/Applications"}
icon = defines["volume_icon"]
badge_icon = None
background = defines["background"]
window_rect = ((200, 120), (540, 380))
default_view = "icon-view"
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
icon_size = 96
text_size = 13
icon_locations = {application_name: (140, 200), "Applications": (400, 200)}

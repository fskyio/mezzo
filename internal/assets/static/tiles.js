/*
 * Optional enhancement for the download menus on the GIF tiles of search
 * results, profiles and related GIFs.
 *
 * The <details> dropdown works on its own. With this script it also closes on
 * outside click, Escape, or after picking a format (a download does not
 * navigate, so the menu would otherwise stay open).
 */
(function () {
	"use strict";

	document.addEventListener("click", function (event) {
		var menus = document.querySelectorAll(".tile-menu[open]");
		for (var i = 0; i < menus.length; i++) {
			var menu = menus[i];
			// A click on the menu's own button is left to toggle it
			if (!menu.contains(event.target) || event.target.closest("li a")) {
				menu.open = false;
			}
		}
	});

	document.addEventListener("keydown", function (event) {
		if (event.key !== "Escape") {
			return;
		}
		var menus = document.querySelectorAll(".tile-menu[open]");
		for (var i = 0; i < menus.length; i++) {
			var menu = menus[i];
			var hadFocus = menu.contains(document.activeElement);
			menu.open = false;
			if (hadFocus) {
				menu.querySelector("summary").focus();
			}
		}
	});
})();

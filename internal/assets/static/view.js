/*
 * Optional enhancement for the SD GIF / HD GIF / MP4 tabs on the view page.
 *
 * Without this script the tabs are plain links to ?format=... and every switch
 * is a full page load. With it, the media is swapped in place. That matters for
 * MP4: the click is a user gesture, so play() is allowed even in browsers that
 * refuse to autoplay on a fresh page load (for example iOS Low Power Mode, or
 * Firefox/Safari set to block autoplay).
 */
(function () {
	"use strict";

	var container = document.querySelector(".view-media");
	var tabs = container && container.querySelector(".tabs");
	if (!tabs) {
		return;
	}

	// Id of the most recent switch. A slower, older switch drops itself.
	var latest = 0;

	function mediaElement() {
		return container.querySelector("img, video");
	}

	function createImage(label) {
		var img = new Image();
		img.alt = label;
		return img;
	}

	function createVideo(label) {
		var video = document.createElement("video");
		video.setAttribute("aria-label", label);
		video.autoplay = true;
		video.loop = true;
		video.muted = true;
		video.playsInline = true;
		video.controls = true;
		return video;
	}

	function discard(element) {
		if (element.tagName === "VIDEO") {
			element.pause();
			element.removeAttribute("src");
			element.load();
		}
	}

	function select(tab) {
		var id = ++latest;
		var current = mediaElement();
		var label = current.getAttribute("alt") || current.getAttribute("aria-label") || "";
		var isVideo = tab.hasAttribute("data-video");
		var next = isVideo ? createVideo(label) : createImage(label);

		var selected = tabs.querySelector(".selected");
		if (selected) {
			selected.classList.remove("selected");
			selected.removeAttribute("aria-current");
		}
		tab.classList.add("selected");
		tab.setAttribute("aria-current", "true");

		function swap() {
			if (id !== latest) {
				discard(next);
				return;
			}
			mediaElement().replaceWith(next);
			history.replaceState(null, "", tab.href);
		}

		function fallback() {
			// Let the server render it
			if (id === latest) {
				window.location.href = tab.href;
			}
		}

		// Keep the old media on screen until the new one can be shown.
		next.addEventListener(isVideo ? "loadedmetadata" : "load", swap);
		next.addEventListener("error", fallback);
		next.src = tab.dataset.src;

		if (isVideo) {
			// Must happen inside the click handler to count as a user gesture
			var started = next.play();
			if (started) {
				started.catch(function () {});
			}
		}
	}

	tabs.addEventListener("click", function (event) {
		var tab = event.target.closest(".tab");
		if (
			!tab ||
			event.defaultPrevented ||
			event.button !== 0 ||
			event.altKey ||
			event.ctrlKey ||
			event.metaKey ||
			event.shiftKey
		) {
			return;
		}

		event.preventDefault();
		if (!tab.classList.contains("selected")) {
			select(tab);
		}
	});
})();

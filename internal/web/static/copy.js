// Copies the YAML of a recipe or member for pasting into the household file.
"use strict";

document.addEventListener("click", async (event) => {
  const button = event.target.closest("[data-copy]");
  if (!button) return;
  const source = document.getElementById(button.dataset.copy);
  if (!source) return;
  try {
    await navigator.clipboard.writeText(source.textContent);
    button.textContent = button.dataset.done;
  } catch {
    /* the text stays visible to select by hand */
  }
});

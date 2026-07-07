// Progressive enhancement for the location picker (events form + locations
// page). Without JS everything still works: the new-location fields are
// revealed by the server-rendered state and the address is checked on save.
(function () {
  var choice = document.getElementById("locationChoice");
  var fields = document.getElementById("new-location-fields");

  function syncFields() {
    if (!choice || !fields) return;
    fields.hidden = choice.value !== "new";
  }
  if (choice && fields) {
    // No-JS fallback: if the fields carry values (form redisplay), show them.
    if (choice.value === "new") fields.hidden = false;
    choice.addEventListener("change", syncFields);
    syncFields();
  } else if (fields) {
    // Locations page has the fields without a select — always visible.
    fields.hidden = false;
  }

  var btn = document.getElementById("check-address-btn");
  var result = document.getElementById("check-address-result");
  var address = document.getElementById("newLocationAddress");
  var name = document.getElementById("newLocationName");
  if (!btn || !address) return;

  btn.addEventListener("click", function () {
    var value = address.value.trim();
    if (!value) {
      result.textContent = "Enter an address first.";
      return;
    }
    result.textContent = "Checking…";
    fetch("/locations/check?address=" + encodeURIComponent(value), {
      headers: { Accept: "application/json" },
    })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        if (!data.ok) {
          result.textContent = "Checker unavailable — it will be checked when you save.";
          return;
        }
        if (!data.found) {
          result.textContent = "✗ Not found — double-check the address (or skip the check on save).";
          return;
        }
        result.textContent = "✓ Found: " + data.matched;
        if (name && !name.value.trim() && data.suggestedName) {
          name.value = data.suggestedName;
        }
      })
      .catch(function () {
        result.textContent = "Checker unavailable — it will be checked when you save.";
      });
  });
})();

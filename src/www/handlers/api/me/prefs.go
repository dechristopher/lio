package me

import (
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/prefs"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// The account's own display preferences (the prefs package): what the site
// shows this player. Scoped to the session like everything else in this group —
// the account written to is always the caller's, and the key must be one the
// site actually stores, so a client cannot write arbitrary rows into a
// preference set.
//
// There is no read endpoint beside it. Preferences are rendered into the page
// that offers the switches, which is every page, so a fetch would only ever
// re-read what the document already carries.

// prefRequest sets one boolean preference.
type prefRequest struct {
	Key string `json:"key"`
	On  bool   `json:"on"`
}

// PrefHandler stores one of the caller's own display preferences.
func PrefHandler(c fiber.Ctx) error {
	acct, ok := account(c)
	if !ok {
		return nil
	}

	var req prefRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(errBody{Error: "malformed request"})
	}

	// An unknown key is the client's mistake, not a storage failure: naming it
	// back is useful to whoever is writing the caller, and reveals nothing.
	if !prefs.Valid(req.Key) {
		return c.Status(fiber.StatusBadRequest).
			JSON(errBody{Error: "unknown preference"})
	}

	if err := prefs.SetFlag(acct.ID, req.Key, req.On); err != nil {
		util.Error(str.CDB, "pref write failed key=%s error=%s", req.Key, err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not save that preference"})
	}

	return c.JSON(fiber.Map{"key": req.Key, "on": req.On})
}

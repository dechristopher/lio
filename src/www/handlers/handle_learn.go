package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/learn"
	"github.com/dechristopher/lio/view"
)

// The /learn tutorial: a hands-on beginner's course. The page is server
// rendered with the whole curriculum's display text inlined (see view.Learn),
// so switching lessons costs nothing; only move legality and the coach's
// verdict need the server, and those go through the API below.

// LearnHandler renders the tutorial at its first lesson.
func LearnHandler(c fiber.Ctx) error {
	return renderLearn(c, learn.Lessons[0].Slug)
}

// LearnLessonHandler renders the tutorial opened at a named lesson, so every
// lesson is a real URL somebody can link to or come back to.
func LearnLessonHandler(c fiber.Ctx) error {
	slug := c.Params("slug")
	if _, ok := learn.BySlug(slug); !ok {
		// an unknown lesson is a stale or mistyped link, not an error worth a
		// 404 page: send the learner to the start of the course
		return c.Redirect().Status(fiber.StatusFound).To("/learn")
	}
	return renderLearn(c, slug)
}

// renderLearn renders the tutorial page opened at the given lesson.
func renderLearn(c fiber.Ctx, slug string) error {
	lesson, _ := learn.BySlug(slug)
	meta := view.PageMeta("Learn to play")
	meta.Description = "Learn to play Octad: a free, hands-on beginner's course. " +
		"The rules, the castling twist, choosing your setup, and your first game."
	return view.Render(c, fiber.StatusOK, view.Learn(meta, lesson))
}

// LearnAPIHandler is the tutorial's move endpoint. The browser has no octad
// rules engine, so every move a learner makes is applied and judged here (see
// the learn package). It is stateless: the request carries the position and the
// step, the response carries the result and the coach's line.
func LearnAPIHandler(c fiber.Ctx) error {
	var req learn.Request
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "malformed request"})
	}
	// bound the free-text fields before they reach the parser: everything here
	// is a short fixed-shape string, so anything longer is not a learner
	if len(req.OFEN) > 100 || len(req.UOI) > 8 || len(req.Deploy) > 8 ||
		len(req.Lesson) > 32 {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "malformed request"})
	}

	resp, err := learn.Do(req)
	if err != nil {
		if errors.Is(err, learn.ErrBadRequest) {
			return c.Status(fiber.StatusUnprocessableEntity).
				JSON(fiber.Map{"error": "invalid move or lesson"})
		}
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": "could not run that lesson step"})
	}
	return c.JSON(resp)
}

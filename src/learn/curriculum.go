package learn

// Lessons comprise the course. Nine lessons in four chapters, ordered so that
// nothing is used before it has been taught: coordinates, then how the pieces
// move, then the three things a chess player will not expect (the castling
// rules, how much promotion matters on a board this small, and choosing your
// own starting setup), then how games end, then a real game against the
// gentlest bot.
//
// Writing guidance for anyone adding to this: the learner is assumed to know
// nothing, including chess. Prompt teaches in a sentence or two and stops
// before the instruction; Action is the instruction alone, one sentence, and
// is what the coach panel accents — so never leave it empty and never repeat
// it in Prompt. Hints give the idea, not the move — the "Show me" button is
// there for the move. Success lines confirm what the learner just proved they
// understood, which is what makes it stick. Keep the copy free of gendered
// pronouns: the opponent is a stranger or a bot, and a piece is a piece
// ("Black has only a king", not "his king"). TestNoGenderedPronouns enforces
// it.
//
// Every step is validated at init and replayed by TestCurriculum, so a wrong
// position or an unsolvable puzzle fails the build.
var Lessons = []Lesson{
	// ---- Chapter 1: getting started ----
	{
		Slug:    "board",
		Title:   "The board",
		Chapter: "Getting started",
		Blurb:   "Sixteen squares, and how to name them",
		Icon:    "▦",
		Kind:    KindDrill,
		Steps: []Step{
			{
				OFEN: standardStart,
				Prompt: "Octad is played on sixteen squares. Every square has a name: " +
					"its column letter (a to d) then its row number (1 to 4).",
				Action: "Click a1 — White's bottom-left corner.",
				Hint:   "Bottom-left corner, from White's side of the board.",
				Success: "That's a1. Columns go left to right, rows go bottom to top. " +
					"This is reversed if playing as Black.",
				Goal:     GoalSelect,
				Targets:  []string{"a1"},
				Solution: []string{"a1"},
			},
			{
				OFEN:     standardStart,
				Prompt:   "Now put the two together.",
				Action:   "Find c3: third column across, third row up.",
				Hint:     "Count three columns from the left, then three rows up.",
				Success:  "Exactly. Letter first, number second — always.",
				Goal:     GoalSelect,
				Targets:  []string{"c3"},
				Solution: []string{"c3"},
			},
			{
				OFEN: standardStart,
				Prompt: "Each player starts with four pieces on their back row: a knight, " +
					"a king, and two pawns.",
				Action:   "Black's knight is in the far corner — click it.",
				Hint:     "The far corner from White's view is d4.",
				Success:  "That's d4. You can read any square on the board now.",
				Goal:     GoalSelect,
				Targets:  []string{"d4"},
				Solution: []string{"d4"},
			},
		},
	},
	{
		Slug:    "pieces",
		Title:   "The pieces",
		Chapter: "Getting started",
		Blurb:   "King, knight, and pawn & how each one moves",
		Icon:    "♞",
		Kind:    KindDrill,
		Steps: []Step{
			{
				// a quiet position built for practice: the black pieces are
				// parked out of reach so the king can be walked around freely
				OFEN: "2kn/4/4/K1PP w - - 0 1",
				Prompt: "The king steps one square in any direction. It is slow, but it is " +
					"the piece you cannot lose.",
				Action:   "Walk it up the left edge to a4.",
				Hint:     "Three steps straight up the a-file: a2, a3, a4.",
				Success:  "That's the king: one square at a time, any direction.",
				Goal:     GoalReach,
				Targets:  []string{"a4"},
				Solo:     true,
				Solution: []string{"a1a2", "a2a3", "a3a4"},
			},
			{
				OFEN: "kn2/4/4/NKPP w - - 0 1",
				Prompt: "The knight jumps in an L: two squares one way, then one square " +
					"across. It is the only piece that can hop over others.",
				Action:   "Land it on d2 — it takes two jumps.",
				Hint:     "Go by way of b3.",
				Success:  "Good. The knight always lands on a different colour square than it left.",
				Goal:     GoalReach,
				Targets:  []string{"d2"},
				Solo:     true,
				Solution: []string{"a1b3", "b3d2"},
			},
			{
				OFEN: standardStart,
				Prompt: "Pawns march straight forward, one square at a time — or two on " +
					"their very first move.",
				Action:  "Push your c-pawn two squares, to c3.",
				Hint:    "Only a pawn that has not moved yet may go two squares.",
				Success: "That is the fastest a pawn will ever travel. Pawns never move backwards.",
				Goal:    GoalReach,
				Targets: []string{"c3"},
			},
		},
	},

	// ---- Chapter 2: the moves that surprise people ----
	{
		Slug:    "capture",
		Title:   "Taking pieces",
		Chapter: "Moves worth knowing",
		Blurb:   "Captures, and the strange one: en passant",
		Icon:    "✕",
		Kind:    KindDrill,
		Steps: []Step{
			{
				// after 1. c2 b3 — the standard opening skirmish, and the same
				// position the /about rules page uses for its far-castle demo
				Setup: []string{"c1c2", "b4b3"},
				Prompt: "Pieces capture by moving onto an enemy piece and taking its place. " +
					"Pawns are the exception: they move straight but capture diagonally.",
				Action:  "Black's pawn on b3 is on your pawn's diagonal — take it.",
				Hint:    "Your pawn on c2 captures diagonally forward, to b3 or d3.",
				Success: "A pawn up. On a board this small, one pawn often decides the game.",
				Goal:    GoalCapture,
				Targets: []string{"b3"},
				// black's answer to 1. c2, and the move that put the pawn on the
				// diagonal this step is about
				PriorMove: "b4b3",
				Solution:  []string{"c2b3"},
			},
			{
				// black has just double-pushed c4-c2 past the white b2 pawn; the
				// en passant square is the c3 it skipped over
				OFEN: "3k/4/1Pp1/K3 w - c3 0 2",
				Prompt: "Black's pawn just rushed two squares to slip past yours. It does not " +
					"get away with it: for this one move only, you may capture it as if it " +
					"had moved a single square. That is en passant.",
				Action: "Capture onto c3.",
				Hint: "Take with your b2 pawn onto the empty square the black pawn skipped, c3. " +
					"The offer expires if you play anything else.",
				Success: "En passant. It is the only capture that lands on an empty square.",
				Goal:    GoalEnPassant,
				// the double push that created the chance: without seeing the
				// two squares the pawn came through, the rule is arbitrary
				PriorMove: "c4c2",
				Solution:  []string{"b2c3"},
			},
		},
	},
	{
		Slug:    "castling",
		Title:   "Castling",
		Chapter: "Moves worth knowing",
		Blurb:   "The big twist: your king castles with anything",
		Icon:    "⇋",
		Kind:    KindDrill,
		Steps: []Step{
			{
				OFEN: standardStart,
				Prompt: "Here is where Octad parts company with chess. Castling lets your king " +
					"trade places with a teammate — and in Octad that teammate can be any of " +
					"your back-row pieces, not just a rook. Your king is on b1 and your knight " +
					"is right beside it on a1.",
				Action:   "Swap them: that is the near castle.",
				Hint:     "Move the king onto your own knight — pieces that stand side by side simply swap.",
				Success:  "That is the near castle, written O. Neither piece had moved yet, which is the one condition.",
				Goal:     GoalCastle,
				Castle:   "near",
				Solution: []string{"b1a1"},
			},
			{
				OFEN: standardStart,
				Prompt: "The three castles are named for how far away the partner sits: near, " +
					"center, and far. Your c1 pawn is the center partner.",
				Action:   "Swap the king with it.",
				Hint:     "Same idea as before, in the other direction: king onto c1.",
				Success:  "The center castle, written O-O. Castling counts as a king move, so you only ever get one per game.",
				Goal:     GoalCastle,
				Castle:   "center",
				Solution: []string{"b1c1"},
			},
			{
				// after 1. c2 b3: the center partner has moved, so only the near
				// and far castles remain — and the far one now has a clear path
				Setup: []string{"c1c2", "b4b3"},
				Prompt: "The far partner is your d1 pawn, and a partner further away does not " +
					"swap — the two cross. The king slides toward the partner and stops one " +
					"square short, and the partner hops to the square just past it. Every " +
					"square between them must be empty, which it now is.",
				Action: "Drag your king onto the d1 pawn to castle far.",
				Hint: "Always castle by moving your king onto the partner itself — here, the d1 " +
					"pawn. The king stops on c1 and the pawn crosses over to b1. Dropping the " +
					"king on the empty c1 is just an ordinary king move, and gives up all " +
					"three castles.",
				Success: "The far castle, written O-O-O. Now you've seen all three castling options.",
				Goal:    GoalCastle,
				Castle:  "far",
				// a castle is encoded king-square to *partner*-square, not to
				// the square the king lands on — so the far castle is b1d1 even
				// though the king stops on c1
				Solution: []string{"b1d1"},
			},
		},
	},
	{
		Slug:    "promotion",
		Title:   "Promotion",
		Chapter: "Moves worth knowing",
		Blurb:   "Queens are earned, not given",
		Icon:    "♛",
		Kind:    KindDrill,
		Steps: []Step{
			{
				OFEN: "k3/2P1/4/K3 w - - 0 1",
				Prompt: "Nobody starts with a queen, rook or bishop in Octad. The only way one " +
					"ever appears is to march a pawn all the way to the far row, where it " +
					"promotes into whichever piece you choose. Your pawn is one square away.",
				Action: "Push it to c4 and choose your new piece.",
				Hint:   "Push c3 to c4, then pick a piece when you are asked — the queen is usually the one you want.",
				// {piece} names whatever they actually chose (see successLine):
				// underpromoting to a knight is a real decision and the coach
				// should not congratulate them on a queen they did not take
				Success:  "A {piece} out of nothing. That fight to push a pawn through is the heart of Octad.",
				Goal:     GoalPromote,
				Solution: []string{"c3c4q"},
			},
		},
	},

	// ---- Chapter 3: how games finish ----
	{
		Slug:    "check",
		Title:   "Check and checkmate",
		Chapter: "Ending the game",
		Blurb:   "Attacking the king, finishing the job",
		Icon:    "⚔",
		Kind:    KindDrill,
		Steps: []Step{
			{
				OFEN: "3k/4/4/K1q1 w - - 0 1",
				Prompt: "A king under attack is in check, and you must answer it immediately — " +
					"you may never leave your own king attacked. Black's queen has yours in " +
					"check along the bottom row. Notice the board offers you exactly one " +
					"escape square: every other one is attacked.",
				Action:  "Play it.",
				Hint:    "Two of your king's three squares are attacked by the queen. Step onto the one that is not.",
				Success: "That is the rule that shapes everything: a move that leaves your king in check is not a move at all.",
				Goal:    GoalEscape,
				// the queen dropping to the bottom row is what gave the check
				PriorMove: "c2c1",
			},
			{
				// exactly one move mates here; the old position for this step
				// had four, which let a guess pass for understanding
				OFEN: "3k/4/1KQ1/4 w - - 0 1",
				Prompt: "Checkmate wins the game: when the king is in check and there is no legal " +
					"way out with no escape square, no way to block, and no way to capture the " +
					"attacker. Black's king is boxed into the corner, and your king already " +
					"covers the square your queen wants.",
				Action: "Find mate in one.",
				Hint: "Look for the one square where your queen attacks the king and both " +
					"squares it could run to — and where your own king defends it, so the " +
					"queen cannot simply be taken.",
				Success: "Checkmate. That's game over.",
				Goal:    GoalMate,
				Moves:   1,
				// the king was checked on c4 and stepped into the corner
				PriorMove: "c4d4",
			},
			{
				OFEN: "3k/PP1p/4/1K2 w - - 0 1",
				Prompt: "Promotion is not only how you get a queen — often it is the mating move " +
					"itself. Black's king is cornered, Black's own pawn is one square from " +
					"promoting too, and one of your pawns can end it first.",
				Action: "Promote, and finish the game.",
				Hint: "Only one of your pawns has a clear run to the far row, and only one " +
					"of the four pieces you may choose gives mate when it arrives.",
				Success: "Mate the instant the queen appeared. A pawn one square from the far " +
					"row is the most dangerous thing on the board.",
				Goal:  GoalMate,
				Moves: 1,
				// black's king was checked on c4 by the b3 pawn and ran for the corner
				PriorMove: "c4d4",
			},
			{
				OFEN: "r3/k1P1/2P1/NK2 w NCF - 0 1",
				Prompt: "One last idea, and it is pure Octad: a castle can be the mating move. " +
					"Neither your king nor your knight has moved, so the near castle is still " +
					"available — and it does not just tuck the king away, it throws the knight " +
					"across the board.",
				Action: "Castle to deliver mate.",
				Hint: "Castle the way you always do — move the king onto the partner itself. " +
					"Work out which square the knight lands on, and what it attacks from there.",
				Success: "Checkmate by castling. The knight lands giving check while your king " +
					"covers the squares Black would run to — one move doing two jobs.",
				Goal:   GoalMate,
				Castle: "near",
				Moves:  1,
				// checked on b4 by the c3 pawn, the king stepped down to a3
				PriorMove: "b4a3",
			},
		},
	},
	{
		Slug:    "draws",
		Title:   "Draws",
		Chapter: "Ending the game",
		Blurb:   "When nobody wins (and how to avoid it)",
		Icon:    "½",
		Kind:    KindDrill,
		Steps: []Step{
			{
				// one move stalemates here; the earlier position for this step
				// had two, so the idea could be found by accident
				OFEN: "2k1/4/1K2/3Q w - - 0 1",
				Prompt: "Not every game has a winner. The trap to know is stalemate: if the " +
					"player to move has no legal move but is not in check, the game is drawn " +
					"on the spot. Black has only a king, on c4.",
				Action: "Take away its last squares without checking it.",
				Hint: "Your king already covers b3. Bring the queen to the one square that " +
					"takes away every other square around the black king — while still not " +
					"attacking the king itself.",
				Success: "Drawn — and with a queen on the board! Watch for this when you are winning: " +
					"check the king or leave it a square, never neither.",
				Goal:  GoalStalemate,
				Moves: 1,
				// the king drifted along the top rank into the trap
				PriorMove: "b4c4",
			},
			{
				// the endgame every beginner reaches and the queen promotion
				// that throws it away: taking a rook instead still wins here
				OFEN: "4/1P1k/1K2/4 w - - 0 1",
				Prompt: "Now the version that actually costs people games. A king and pawn " +
					"against a lone king is the endgame you will reach most often, and the " +
					"greedy move loses the win: Black's king is hemmed against the edge, and " +
					"your pawn is one square from promoting.",
				Action: "Promote to a queen and watch what happens.",
				Hint: "Push the pawn to the last row and take the strongest piece. Then count " +
					"the squares the black king has left.",
				Success: "Drawn — the new queen covers every square the king could use, and " +
					"leaves it none. Taking a rook instead would have kept the win. That is " +
					"why strong players sometimes promote to something smaller.",
				Goal:  GoalStalemate,
				Moves: 1,
				// the king was pushed off c3 and is now hemmed against the edge
				PriorMove: "c3d3",
			},
		},
	},

	// ---- Chapter 4: a real game ----
	{
		Slug:    "deploy",
		Title:   "Choose your setup",
		Chapter: "Playing for real",
		Blurb:   "Arrange your own pieces before the game starts",
		Icon:    "⇄",
		Kind:    KindDeploy,
		Steps: []Step{
			{
				OFEN: standardStart,
				Prompt: "One more twist, and it happens before the first move. In Octad games " +
					"both players privately arrange their four pieces along their own back " +
					"row, then the setups are revealed together. Every rule adapts, castling " +
					"included, since near, center, and far follow wherever your king ends up.",
				Action: "Drag your pieces into any order you like, then commit.",
				Hint: "There is no wrong answer. A king in a corner is harder to reach; a king " +
					"in the middle castles both ways immediately.",
				Success: "That is your setup, and Black's is revealed beside it. Neither of you " +
					"saw the other's while choosing.",
				Goal: GoalDeploy,
			},
		},
	},
	{
		Slug:    "play",
		Title:   "Your first game",
		Chapter: "Playing for real",
		Blurb:   "Put it all together against the gentlest bot",
		Icon:    "▶",
		Kind:    KindPlay,
		Steps: []Step{
			{
				OFEN: standardStart,
				Prompt: "That is everything. You play White against Pawn, the gentlest bot on " +
					"the site — it knows the rules and not much else. There is no clock, and " +
					"you can reset whenever you like.",
				Action: "Win a game!",
				Hint: "Push a pawn, protect your king, and take anything Black leaves hanging. " +
					"A single promoted queen usually ends it.",
				Success: "You won! That's a complete game of Octad. You are ready for a real one.",
				Goal:    GoalWin,
				Engine:  true,
			},
		},
	},
}

// Chapter groups lessons for the rail: a chapter title and the lessons under it,
// in course order.
type Chapter struct {
	Title   string
	Lessons []*Lesson
}

// Chapters returns the course grouped by chapter, preserving the order lessons
// are declared in.
func Chapters() []Chapter {
	var out []Chapter
	for i := range Lessons {
		l := &Lessons[i]
		if len(out) == 0 || out[len(out)-1].Title != l.Chapter {
			out = append(out, Chapter{Title: l.Chapter})
		}
		out[len(out)-1].Lessons = append(out[len(out)-1].Lessons, l)
	}
	return out
}

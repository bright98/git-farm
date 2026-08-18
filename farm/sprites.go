package farm

// A Sprite is pixel art written as text. Each byte in Art is looked up in Key
// to get a Role, and the Theme turns that role into a colour. A dot means
// "leave what is already there".
type Sprite struct {
	Art []string
	Key map[byte]Role
}

func (s Sprite) H() int { return len(s.Art) }

func (s Sprite) W() int {
	w := 0
	for _, row := range s.Art {
		if len(row) > w {
			w = len(row)
		}
	}
	return w
}

func (c *Canvas) Blit(s Sprite, x, y int, t *Theme) {
	for dy, row := range s.Art {
		for dx := 0; dx < len(row); dx++ {
			role, ok := s.Key[row[dx]]
			if !ok {
				continue
			}
			c.Set(x+dx, y+dy, t.Colour(role))
		}
	}
}

// The same letters mean the same thing in every sprite.
var key = map[byte]Role{
	'H': RoleLeafHi, 'L': RoleLeaf, 'S': RoleStem,
	'W': RoleWeed, 'G': RoleWeedHi,
	'D': RoleDry, 'T': RoleDryStem,
	'C': RoleHat, 'K': RoleSkin, 'B': RoleShirt, 'P': RolePants, 'O': RoleBoots,
}

// ---- the full set: leafy, round, a little world ------------------------

var fullSprites = SpriteSet{
	Plant: Sprite{Key: key, Art: []string{
		"..H..",
		".HLH.",
		"HLLLH",
		".HLH.",
		"..S..",
		"..S..",
	}},
	Tall: Sprite{Key: key, Art: []string{
		"..H..",
		".HLH.",
		"HLLLH",
		"HLLLH",
		".HLH.",
		"..S..",
		"..S..",
		".S.S.",
		"..S..",
	}},
	Weed: Sprite{Key: key, Art: []string{
		"W...W",
		".W.W.",
		"..W..",
		".WGW.",
		"..W..",
		"..W..",
	}},
	Dry: Sprite{Key: key, Art: []string{
		".....",
		"..D..",
		".D.D.",
		"..D..",
		"..T..",
		"..T..",
	}},
	Farmer: Sprite{Key: key, Art: []string{
		"..CCC..",
		".CCCCC.",
		"CCCCCCC",
		"..KKK..",
		"..KKK..",
		".BBBBB.",
		"KBBBBBK",
		".BBBBB.",
		"..P.P..",
		"..P.P..",
		"..O.O..",
	}},
	FarmerWalk: Sprite{Key: key, Art: []string{
		"..CCC..",
		".CCCCC.",
		"CCCCCCC",
		"..KKK..",
		"..KKK..",
		".BBBBB.",
		"KBBBBBK",
		".BBBBB.",
		"..PP...",
		".P..P..",
		".O...O.",
	}},
}

// ---- the quiet set: the same shapes with the fat trimmed off -----------
//
// Each sprite keeps its silhouette but drops the filled body, so the terminal
// shows through the middle of everything.
//
// Two rules hold these apart, because colour cannot. The quiet palette's green,
// amber and purple sit at almost the same lightness — in greyscale they are one
// shade — so hue tells you nothing a colour-blind reader can use.
//
//	shape  a live plant stands up, a weed sprawls, a dead one bends over
//	ink    4 pixels for dead, 6 for live, 9 for churn
//
// Squint at a field and the weedy one is visibly denser, the dead one visibly
// emptier, before any single shape resolves. Both rules are tested.

var quietSprites = SpriteSet{
	Plant: Sprite{Key: key, Art: []string{ // 6 pixels: upright
		"..H..",
		".HLH.",
		"..S..",
		"..S..",
	}},
	Tall: Sprite{Key: key, Art: []string{ // the same plant, grown
		"..H..",
		".HLH.",
		"HLLLH",
		"..S..",
		"..S..",
		"..S..",
	}},
	Weed: Sprite{Key: key, Art: []string{ // 9 pixels: sprawling, two stems
		"W.W.W",
		".WWW.",
		".W.W.",
		"..W..",
	}},
	Dry: Sprite{Key: key, Art: []string{ // 4 pixels, and it bends over
		"..D..",
		"..T..",
		".T...",
		".T...",
	}},
	Farmer: Sprite{Key: key, Art: []string{
		"..C..",
		".CCC.",
		"..K..",
		".BBB.",
		".BBB.",
		".P.P.",
		".O.O.",
	}},
	FarmerWalk: Sprite{Key: key, Art: []string{
		"..C..",
		".CCC.",
		"..K..",
		".BBB.",
		".BBB.",
		"..PP.",
		".O..O",
	}},
}

// ---- the set for a file, rather than for a terminal --------------------
//
// The quiet set above is sparse because it has to be: it sits on top of
// somebody's session and must not cover it. An SVG has nothing behind it, and
// it is looked at from further away than a terminal, so the same rules can be
// spent on a fuller shape.
//
// The rules do not change, only the room to obey them in:
//
//	shape  a live plant stands up and opens out, a weed sprawls and knots,
//	       a dead one bends over
//	ink    5 pixels for dead, 12 for live, 18 for churn
//
// Both are tested for this set exactly as they are for the quiet one.
var svgSprites = SpriteSet{
	Plant: Sprite{Key: key, Art: []string{ // 14 pixels: leafy, upright
		"..H..",
		".HLH.",
		"HLLLH",
		".HLH.",
		"..S..",
		"..S..",
	}},
	Tall: Sprite{Key: key, Art: []string{ // the same plant, grown
		"..H..",
		".HLH.",
		"HLLLH",
		"HLLLH",
		".HLH.",
		"..S..",
		"..S..",
		".S.S.",
		"..S..",
	}},
	Weed: Sprite{Key: key, Art: []string{ // 18 pixels: knotted, and two stems
		"W.W.W",
		".WWW.",
		"WWGWW",
		".WGW.",
		".W.W.",
		"W...W",
	}},
	Dry: Sprite{Key: key, Art: []string{ // 5 pixels, and it bends over
		".....",
		"..D..",
		".D...",
		".T...",
		".T...",
		".T...",
	}},
	Farmer:     fullSprites.Farmer,
	FarmerWalk: fullSprites.FarmerWalk,
}

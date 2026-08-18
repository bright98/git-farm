package farm

import (
	"math"
	"sort"
)

type Rect struct{ X, Y, W, H int }

func (r Rect) Area() int { return r.W * r.H }

// Squarify lays weighted items out inside an area as rectangles whose sizes are
// proportional to their weights and whose shapes are as close to square as the
// weights allow. It is the treemap from Bruls, Huizing and van Wijk, and it is
// the right shape here for a simple reason: fields are rectangles already.
//
// The returned rectangles are in the same order as the weights, and they touch.
// The gutter between fields is applied afterwards, by inset.
//
// The layout is computed once, from the state at HEAD. Recomputing it per frame
// would make every new file re-flow the whole farm and every field jump, which
// turns a time-lapse into noise; instead, fields keep their place forever and a
// long-deleted file simply leaves its patch of soil empty.
func Squarify(area Rect, weights []float64) []Rect {
	out := make([]Rect, len(weights))
	if len(weights) == 0 || area.W <= 0 || area.H <= 0 {
		return out
	}

	// Biggest first: the algorithm depends on it, and it also means the field
	// with the most files lands in the top-left corner, where the eye starts.
	order := make([]int, len(weights))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return weights[order[a]] > weights[order[b]] })

	total := 0.0
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}
	if total <= 0 {
		return out
	}

	scale := float64(area.Area()) / total
	free := frect{float64(area.X), float64(area.Y), float64(area.W), float64(area.H)}

	var row []float64
	var rowIdx []int

	for i := 0; i < len(order); {
		v := math.Max(weights[order[i]], 0) * scale
		side := math.Min(free.w, free.h)

		// Add the next item to the current row while doing so keeps the row's
		// worst aspect ratio from getting worse. When it would, close the row.
		if len(row) == 0 || worst(append(append([]float64{}, row...), v), side) <= worst(row, side) {
			row = append(row, v)
			rowIdx = append(rowIdx, order[i])
			i++
			continue
		}

		free = layoutRow(row, rowIdx, free, area, out)
		row, rowIdx = nil, nil
	}
	if len(row) > 0 {
		layoutRow(row, rowIdx, free, area, out)
	}
	return out
}

type frect struct{ x, y, w, h float64 }

// worst is the worst aspect ratio in a row laid along a side of length side.
func worst(row []float64, side float64) float64 {
	if len(row) == 0 || side <= 0 {
		return math.Inf(1)
	}
	sum, lo, hi := 0.0, math.Inf(1), 0.0
	for _, v := range row {
		sum += v
		lo = math.Min(lo, v)
		hi = math.Max(hi, v)
	}
	if sum <= 0 || lo <= 0 {
		return math.Inf(1)
	}
	s2, side2 := sum*sum, side*side
	return math.Max(side2*hi/s2, s2/(side2*lo))
}

// layoutRow places one row along the shorter side of free and returns what is
// left of it.
func layoutRow(row []float64, idx []int, free frect, area Rect, out []Rect) frect {
	sum := 0.0
	for _, v := range row {
		sum += v
	}
	if sum <= 0 {
		return free
	}

	if free.w <= free.h {
		// A horizontal strip across the top: items side by side.
		h := sum / free.w
		x := free.x
		for i, v := range row {
			w := v / h
			out[idx[i]] = clamp(frect{x, free.y, w, h}, area)
			x += w
		}
		return frect{free.x, free.y + h, free.w, free.h - h}
	}

	// A vertical strip down the left: items stacked.
	w := sum / free.h
	y := free.y
	for i, v := range row {
		h := v / w
		out[idx[i]] = clamp(frect{free.x, y, w, h}, area)
		y += h
	}
	return frect{free.x + w, free.y, free.w - w, free.h}
}

// clamp rounds a float rectangle to whole pixels without letting it escape the
// area it was laid out in.
func clamp(f frect, area Rect) Rect {
	x0 := int(math.Round(f.x))
	y0 := int(math.Round(f.y))
	x1 := int(math.Round(f.x + f.w))
	y1 := int(math.Round(f.y + f.h))

	x0 = maxInt(x0, area.X)
	y0 = maxInt(y0, area.Y)
	x1 = minInt(x1, area.X+area.W)
	y1 = minInt(y1, area.Y+area.H)

	return Rect{X: x0, Y: y0, W: maxInt(0, x1-x0), H: maxInt(0, y1-y0)}
}

// Inset opens a gutter between neighbouring fields.
//
// The vertical numbers are not the horizontal ones. A pixel is half a cell
// tall, and a field's border is drawn in the character overlay, one rune per
// cell — so two fields whose borders land in the same cell row would overwrite
// each other. Snapping to even pixels and leaving two of them free puts a whole
// empty cell row between any two fields.
func Inset(r Rect) Rect {
	r.X++
	r.W -= 2

	if r.Y%2 == 1 {
		r.Y++
		r.H--
	}
	r.H -= 2
	r.H -= r.H % 2

	if r.W < 0 {
		r.W = 0
	}
	if r.H < 0 {
		r.H = 0
	}
	return r
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
